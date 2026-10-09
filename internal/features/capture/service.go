// internal/features/capture/service.go
package capture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
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
	ClaimSession(ctx context.Context, pairingCode string) (*ClaimedSession, error)
	AuthenticateMobile(ctx context.Context, uploadToken string) (*MobileCredential, error)
	MobileHeartbeat(ctx context.Context, credential MobileCredential, sessionID uuid.UUID) (*SessionState, error)
	UploadCapture(ctx context.Context, credential MobileCredential, input UploadInput) (*capturedomain.Capture, error)
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

type ClaimedSession struct {
	SessionID            uuid.UUID
	UploadToken          string
	UploadTokenExpiresAt time.Time
}

type MobileCredential struct {
	SessionID uuid.UUID
	AccountID uuid.UUID
	tokenHash []byte
}

type UploadInput struct {
	File             io.ReadSeeker
	OriginalFilename string
	SizeBytes        int64
}

type pairingCodeGenerator interface {
	Generate() (plain string, hash []byte, err error)
}

type clock func() time.Time

type service struct {
	repository Repository
	storage    FileStorage
	codes      pairingCodeGenerator
	now        clock
}

var _ Service = (*service)(nil)

func New(repository Repository, storage FileStorage) Service {
	return newService(repository, storage, securePairingCodeGenerator{}, time.Now)
}

func newService(repository Repository, storage FileStorage, codes pairingCodeGenerator, now clock) Service {
	return &service{repository: repository, storage: storage, codes: codes, now: now}
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

func (s *service) ClaimSession(ctx context.Context, pairingCode string) (*ClaimedSession, error) {
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}
	pairingCode = strings.TrimSpace(pairingCode)
	if pairingCode == "" {
		return nil, invalidCaptureCredential("código de pareamento inválido ou expirado", nil)
	}

	uploadToken, uploadTokenHash, err := s.codes.Generate()
	if err != nil {
		return nil, apperr.Internal("erro inesperado", fmt.Errorf("generate capture upload token: %w", err))
	}
	now := s.now().UTC()
	claim, err := capturedomain.NewSessionClaim(uploadTokenHash, now)
	if err != nil {
		return nil, apperr.Internal("erro inesperado", fmt.Errorf("create capture session claim: %w", err))
	}
	pairingHash := sha256.Sum256([]byte(pairingCode))
	session, err := s.repository.ClaimSession(ctx, pairingHash[:], claim, now.Add(-DesktopPresenceWindow))
	if err != nil {
		return nil, mapClaimError(err)
	}
	return &ClaimedSession{
		SessionID:            session.ID,
		UploadToken:          uploadToken,
		UploadTokenExpiresAt: claim.UploadTokenExpiresAt,
	}, nil
}

func (s *service) AuthenticateMobile(ctx context.Context, uploadToken string) (*MobileCredential, error) {
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}
	uploadToken = strings.TrimSpace(uploadToken)
	if uploadToken == "" {
		return nil, apperr.Unauthorized("credencial de captura necessária")
	}
	tokenHash := sha256.Sum256([]byte(uploadToken))
	session, err := s.repository.AuthenticateMobile(ctx, tokenHash[:], s.now().UTC())
	if err != nil {
		return nil, mapMobileAuthenticationError(err)
	}
	return &MobileCredential{
		SessionID: session.ID,
		AccountID: session.AccountID,
		tokenHash: cloneBytes(tokenHash[:]),
	}, nil
}

func (s *service) MobileHeartbeat(ctx context.Context, credential MobileCredential, sessionID uuid.UUID) (*SessionState, error) {
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}
	if err := credential.validate(); err != nil {
		return nil, err
	}
	if sessionID == uuid.Nil || sessionID != credential.SessionID {
		return nil, apperr.NotFound("sessão de captura não encontrada")
	}
	now := s.now().UTC()
	if err := s.repository.TouchMobile(ctx, sessionID, credential.tokenHash, now); err != nil {
		return nil, mapMobileAuthenticationError(err)
	}
	session, err := s.repository.FindSession(ctx, sessionID)
	if err != nil {
		return nil, mapRepositoryError("captureRepository.FindSession", err)
	}
	state := sessionState(*session, now)
	return &state, nil
}

func (s *service) UploadCapture(ctx context.Context, credential MobileCredential, input UploadInput) (*capturedomain.Capture, error) {
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, apperr.Internal("armazenamento de capturas indisponível", errors.New("capture storage is not configured"))
	}
	if err := credential.validate(); err != nil {
		return nil, err
	}
	if input.File == nil {
		return nil, apperr.Validation("arquivo é obrigatório", apperr.Violation{Field: "file", Reason: "required"})
	}
	if input.SizeBytes <= 0 {
		return nil, apperr.Validation("arquivo vazio", apperr.Violation{Field: "file", Reason: "empty"})
	}
	if input.SizeBytes > capturedomain.MaxFileSizeBytes {
		return nil, &apperr.AppError{Kind: apperr.UPLOAD_SIZE_EXCEEDED, Message: "o arquivo deve ter no máximo 5 MiB"}
	}

	filename, err := normalizedFilename(input.OriginalFilename)
	if err != nil {
		return nil, err
	}
	mimeType, extension, err := detectCaptureFileType(input.File)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	session, err := s.repository.AuthenticateUpload(ctx, credential.tokenHash, now, now.Add(-DesktopPresenceWindow))
	if err != nil {
		return nil, mapMobileAuthenticationError(err)
	}
	if session.ID != credential.SessionID || session.AccountID != credential.AccountID {
		return nil, invalidCaptureCredential("credencial de captura inválida ou expirada", errors.New("authenticated capture session changed"))
	}

	item, err := capturedomain.NewCapture(capturedomain.NewCaptureParams{
		AccountID:        session.AccountID,
		CaptureSessionID: session.ID,
		OriginalFilename: filename,
		MIMEType:         mimeType,
		SizeBytes:        input.SizeBytes,
		CreatedAt:        now,
	})
	if err != nil {
		return nil, apperr.Internal("erro inesperado", fmt.Errorf("create capture domain model: %w", err))
	}
	objectName := fmt.Sprintf("%s/%s.%s", item.AccountID, item.ID, extension)
	expectedURI, err := s.storage.ObjectURI(objectName)
	if err != nil {
		return nil, mapStorageError("captureStorage.ObjectURI", err)
	}
	if err := s.repository.CreateCapture(ctx, item); err != nil {
		return nil, mapRepositoryError("captureRepository.CreateCapture", err)
	}

	storageURI, err := s.storage.Upload(ctx, input.File, objectName, mimeType)
	if err != nil {
		return nil, s.compensateCapture(ctx, item.ID, expectedURI, now, mapStorageError("captureStorage.Upload", err))
	}
	available, err := s.repository.SetCaptureAvailable(ctx, item.ID, storageURI, s.now().UTC())
	if err != nil {
		return nil, s.compensateCapture(ctx, item.ID, storageURI, s.now().UTC(), mapRepositoryError("captureRepository.SetCaptureAvailable", err))
	}
	return available, nil
}

func (s *service) validateDependencies() error {
	if s == nil || s.repository == nil || s.codes == nil || s.now == nil {
		return apperr.Internal("erro inesperado", errors.New("capture service is not configured"))
	}
	return nil
}

func (c MobileCredential) validate() error {
	if c.SessionID == uuid.Nil || c.AccountID == uuid.Nil || len(c.tokenHash) != capturedomain.CredentialHashBytes {
		return invalidCaptureCredential("credencial de captura inválida ou expirada", errors.New("invalid mobile credential context"))
	}
	return nil
}

func normalizedFilename(value string) (string, error) {
	filename := path.Base(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	if filename == "" || filename == "." || len(filename) > 255 || strings.ContainsAny(filename, "\x00\r\n") {
		return "", apperr.Validation("nome do arquivo inválido", apperr.Violation{Field: "file", Reason: "invalid_filename"})
	}
	return filename, nil
}

func detectCaptureFileType(file io.ReadSeeker) (mimeType, extension string, resultErr error) {
	header := make([]byte, 512)
	read, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", apperr.Internal("falha ao ler arquivo", fmt.Errorf("read capture signature: %w", err))
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", apperr.Internal("falha ao ler arquivo", fmt.Errorf("reset capture file: %w", err))
	}
	header = header[:read]
	switch {
	case bytes.HasPrefix(header, []byte("%PDF-")):
		return "application/pdf", "pdf", nil
	case bytes.HasPrefix(header, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg", "jpg", nil
	case bytes.HasPrefix(header, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png", "png", nil
	default:
		return "", "", &apperr.AppError{Kind: apperr.UNSUPPORTED_MEDIA_TYPE, Message: "envie um arquivo PDF, JPEG ou PNG válido"}
	}
}

func (s *service) compensateCapture(ctx context.Context, captureID uuid.UUID, storageURI string, at time.Time, primary error) error {
	var compensationErrors []error
	if _, err := s.repository.MarkCaptureDeleting(ctx, captureID, at); err != nil {
		compensationErrors = append(compensationErrors, mapRepositoryError("captureRepository.MarkCaptureDeleting", err))
	}
	if err := s.storage.Delete(ctx, storageURI); err != nil {
		compensationErrors = append(compensationErrors, mapStorageError("captureStorage.Delete", err))
	}
	return withAdditionalCauses(primary, compensationErrors...)
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

func mapClaimError(err error) error {
	if errors.Is(err, ErrStateConflict) || errors.Is(err, ErrSessionNotFound) {
		return invalidCaptureCredential("código de pareamento inválido ou expirado", err)
	}
	return mapRepositoryError("captureRepository.ClaimSession", err)
}

func mapMobileAuthenticationError(err error) error {
	if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrStateConflict) {
		return invalidCaptureCredential("credencial de captura inválida ou expirada", err)
	}
	return mapRepositoryError("captureRepository.AuthenticateMobile", err)
}

func invalidCaptureCredential(message string, cause error) error {
	return &apperr.AppError{Kind: apperr.AUTH_TOKEN_INVALID, Message: message, Cause: cause}
}

func mapStorageError(operation string, err error) error {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr != nil {
		return appErr
	}
	return &apperr.AppError{
		Kind:    apperr.INFRA_STORAGE_ERROR,
		Message: "falha no armazenamento de capturas",
		Cause:   fmt.Errorf("%s: %w", operation, err),
	}
}

func withAdditionalCauses(primary error, additional ...error) error {
	joined := errors.Join(additional...)
	if joined == nil {
		return primary
	}
	var appErr *apperr.AppError
	if errors.As(primary, &appErr) && appErr != nil {
		result := *appErr
		result.Violations = append([]apperr.Violation(nil), appErr.Violations...)
		result.Cause = errors.Join(appErr.Cause, joined)
		return &result
	}
	return apperr.Internal("erro inesperado", errors.Join(primary, joined))
}

func cloneBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
