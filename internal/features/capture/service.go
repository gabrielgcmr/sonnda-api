// internal/features/capture/service.go
package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
	"github.com/google/uuid"
)

const DesktopPresenceWindow = time.Minute

type Service interface {
	CreateSession(ctx context.Context, accountID uuid.UUID) (*CreatedSession, error)
	CurrentSession(ctx context.Context, accountID uuid.UUID) (*SessionState, error)
	Heartbeat(ctx context.Context, accountID, sessionID uuid.UUID) (*SessionState, error)
	RevokeSession(ctx context.Context, accountID, sessionID uuid.UUID) error
}

type CreatedSession struct {
	SessionState
	PairingCode string
}

type SessionState struct {
	ID                   uuid.UUID
	PairingExpiresAt     time.Time
	ClaimedAt            *time.Time
	UploadTokenExpiresAt *time.Time
	DesktopLastSeenAt    time.Time
	MobileLastSeenAt     *time.Time
	DesktopPresent       bool
	MobilePresent        bool
	Connected            bool
}

type pairingCodeGenerator interface {
	Generate() (plain string, hash []byte, err error)
}

type clock func() time.Time

type service struct {
	repository Repository
	codes      pairingCodeGenerator
	now        clock
}

var _ Service = (*service)(nil)

func New(repository Repository) Service {
	return newService(repository, securePairingCodeGenerator{}, time.Now)
}

func newService(repository Repository, codes pairingCodeGenerator, now clock) Service {
	return &service{repository: repository, codes: codes, now: now}
}

func (s *service) CreateSession(ctx context.Context, accountID uuid.UUID) (*CreatedSession, error) {
	if accountID == uuid.Nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}

	pairingCode, pairingHash, err := s.codes.Generate()
	if err != nil {
		return nil, apperr.Internal("erro inesperado", fmt.Errorf("generate capture pairing code: %w", err))
	}
	now := s.now().UTC()
	session, err := capturedomain.NewSession(accountID, pairingHash, now)
	if err != nil {
		return nil, apperr.Internal("erro inesperado", fmt.Errorf("create capture session domain model: %w", err))
	}

	err = s.repository.WithinTransaction(ctx, func(repository Repository) error {
		if _, err := repository.RevokeSessionsByAccount(ctx, accountID, now); err != nil {
			return err
		}
		return repository.CreateSession(ctx, session)
	})
	if err != nil {
		return nil, mapRepositoryError("captureRepository.CreateSession", err)
	}

	return &CreatedSession{
		SessionState: sessionState(session, now),
		PairingCode:  pairingCode,
	}, nil
}

func (s *service) CurrentSession(ctx context.Context, accountID uuid.UUID) (*SessionState, error) {
	if accountID == uuid.Nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}
	session, err := s.repository.FindCurrentSession(ctx, accountID)
	if err != nil {
		return nil, mapRepositoryError("captureRepository.FindCurrentSession", err)
	}
	state := sessionState(*session, s.now().UTC())
	return &state, nil
}

func (s *service) Heartbeat(ctx context.Context, accountID, sessionID uuid.UUID) (*SessionState, error) {
	if accountID == uuid.Nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}
	if sessionID == uuid.Nil {
		return nil, apperr.NotFound("sessão de captura não encontrada")
	}
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if err := s.repository.TouchDesktop(ctx, sessionID, accountID, now); err != nil {
		return nil, mapRepositoryError("captureRepository.TouchDesktop", err)
	}
	session, err := s.repository.FindSession(ctx, sessionID)
	if err != nil {
		return nil, mapRepositoryError("captureRepository.FindSession", err)
	}
	state := sessionState(*session, now)
	return &state, nil
}

func (s *service) RevokeSession(ctx context.Context, accountID, sessionID uuid.UUID) error {
	if accountID == uuid.Nil {
		return apperr.Unauthorized("autenticação necessária")
	}
	if sessionID == uuid.Nil {
		return apperr.NotFound("sessão de captura não encontrada")
	}
	if err := s.validateDependencies(); err != nil {
		return err
	}
	if err := s.repository.RevokeSession(ctx, sessionID, accountID, s.now().UTC()); err != nil {
		return mapRepositoryError("captureRepository.RevokeSession", err)
	}
	return nil
}

func (s *service) validateDependencies() error {
	if s == nil || s.repository == nil || s.codes == nil || s.now == nil {
		return apperr.Internal("erro inesperado", errors.New("capture service is not configured"))
	}
	return nil
}

func sessionState(session capturedomain.Session, now time.Time) SessionState {
	cutoff := now.Add(-DesktopPresenceWindow)
	desktopPresent := session.RevokedAt == nil && !session.DesktopLastSeenAt.Before(cutoff)
	mobilePresent := session.RevokedAt == nil && session.ClaimedAt != nil &&
		session.UploadTokenExpiresAt != nil && session.UploadTokenExpiresAt.After(now) &&
		session.MobileLastSeenAt != nil && !session.MobileLastSeenAt.Before(cutoff)
	return SessionState{
		ID:                   session.ID,
		PairingExpiresAt:     session.PairingExpiresAt,
		ClaimedAt:            cloneTime(session.ClaimedAt),
		UploadTokenExpiresAt: cloneTime(session.UploadTokenExpiresAt),
		DesktopLastSeenAt:    session.DesktopLastSeenAt,
		MobileLastSeenAt:     cloneTime(session.MobileLastSeenAt),
		DesktopPresent:       desktopPresent,
		MobilePresent:        mobilePresent,
		Connected:            desktopPresent && mobilePresent,
	}
}

func mapRepositoryError(operation string, err error) error {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr != nil {
		return appErr
	}
	switch {
	case errors.Is(err, ErrSessionNotFound):
		return &apperr.AppError{Kind: apperr.NOT_FOUND, Message: "sessão de captura não encontrada", Cause: err}
	case errors.Is(err, ErrStateConflict):
		return &apperr.AppError{Kind: apperr.RESOURCE_CONFLICT, Message: "estado da sessão de captura foi alterado", Cause: err}
	case errors.Is(err, persistence.ErrPersistenceFailure):
		return &apperr.AppError{Kind: apperr.INFRA_DATABASE_ERROR, Message: "falha técnica", Cause: fmt.Errorf("%s: %w", operation, err)}
	default:
		return apperr.Internal("erro inesperado", fmt.Errorf("%s: %w", operation, err))
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
