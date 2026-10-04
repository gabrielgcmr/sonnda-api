// internal/features/patient/access/self_identity.go
package patientaccess

import (
	"context"
	"errors"
	"fmt"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

var (
	ErrSelfIdentityConflict = errors.New("self identity conflict")
	ErrSelfIdentityAbsent   = errors.New("self identity absent")
	ErrSelfIdentityTarget   = errors.New("self identity target absent")
	ErrSelfIdentityDenied   = errors.New("self identity professional denied")
)

// SelfIdentityRepository stores professionally confirmed identities. Legacy
// owner_user_id and relation_type=self are never interpreted as confirmation.
type SelfIdentityRepository interface {
	HasActiveSelf(ctx context.Context, patientID, accountID uuid.UUID) (bool, error)
	ConfirmSelf(ctx context.Context, patientID, accountID, professionalID uuid.UUID) error
	RevokeSelf(ctx context.Context, patientID, accountID, professionalID uuid.UUID) error
}

type AccountLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*accountdomain.User, error)
}

type SelfIdentityService struct {
	accounts AccountLookup
	access   Checker
	identity SelfIdentityRepository
}

func NewSelfIdentityService(accounts AccountLookup, access Checker, identity SelfIdentityRepository) *SelfIdentityService {
	return &SelfIdentityService{accounts: accounts, access: access, identity: identity}
}

func (s *SelfIdentityService) Confirm(ctx context.Context, professionalID, patientID, accountID uuid.UUID) error {
	if err := s.requireProfessional(ctx, professionalID, patientID, accountID); err != nil {
		return err
	}
	if professionalID == accountID {
		return apperr.Forbidden("acesso negado")
	}
	target, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return selfIdentityStorageError("accounts.FindByID(target)", err)
	}
	if target == nil || target.ID != accountID {
		return apperr.NotFound("conta não encontrada")
	}
	return mapSelfIdentityError("identity.ConfirmSelf", s.identity.ConfirmSelf(ctx, patientID, accountID, professionalID))
}

func (s *SelfIdentityService) Revoke(ctx context.Context, professionalID, patientID, accountID uuid.UUID) error {
	if err := s.requireProfessional(ctx, professionalID, patientID, accountID); err != nil {
		return err
	}
	return mapSelfIdentityError("identity.RevokeSelf", s.identity.RevokeSelf(ctx, patientID, accountID, professionalID))
}

func (s *SelfIdentityService) requireProfessional(ctx context.Context, professionalID, patientID, accountID uuid.UUID) error {
	if professionalID == uuid.Nil {
		return apperr.Unauthorized("autenticação necessária")
	}
	if patientID == uuid.Nil || accountID == uuid.Nil {
		return apperr.Forbidden("acesso negado")
	}
	if s == nil || s.accounts == nil || s.access == nil || s.identity == nil {
		return apperr.Internal("erro inesperado", errors.New("self identity dependencies not configured"))
	}
	professional, err := s.accounts.FindByID(ctx, professionalID)
	if err != nil {
		return selfIdentityStorageError("accounts.FindByID(professional)", err)
	}
	if professional == nil || professional.ID != professionalID || professional.AccountType != accountdomain.AccountTypeProfessional {
		return apperr.Forbidden("acesso negado")
	}
	return s.access.RequireAccess(ctx, professionalID, patientID)
}

func mapSelfIdentityError(operation string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrSelfIdentityConflict):
		return apperr.Conflict("vínculo de identidade em conflito")
	case errors.Is(err, ErrSelfIdentityAbsent):
		return apperr.NotFound("vínculo de identidade não encontrado")
	case errors.Is(err, ErrSelfIdentityTarget):
		return apperr.NotFound("conta ou paciente não encontrado")
	case errors.Is(err, ErrSelfIdentityDenied):
		return apperr.Forbidden("acesso negado")
	default:
		return selfIdentityStorageError(operation, err)
	}
}

func selfIdentityStorageError(operation string, err error) error {
	return &apperr.AppError{Kind: apperr.INFRA_DATABASE_ERROR, Message: "falha técnica", Cause: fmt.Errorf("%s: %w", operation, err)}
}
