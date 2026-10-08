// internal/features/patient/profile/service.go
package patientprofile

import (
	"context"
	"errors"
	"fmt"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"

	"github.com/google/uuid"
)

type Service interface {
	Get(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*profiledomain.Patient, error)
	Update(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID, input UpdateInput) (*profiledomain.Patient, error)
	SoftDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error
	HardDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error
	ListMyPatients(ctx context.Context, currentAccount *accountdomain.Account, limit, offset int) ([]*profiledomain.Patient, error)
}

type AccessChecker interface {
	RequireAccess(ctx context.Context, accountID, patientID uuid.UUID) error
}

type service struct {
	repo          Repository
	accessRepo    patientaccess.Repository
	accessChecker AccessChecker
}

var _ Service = (*service)(nil)

func New(
	repo Repository,
	accessRepo patientaccess.Repository,
	accessChecker AccessChecker,
) Service {
	return &service{
		repo:          repo,
		accessRepo:    accessRepo,
		accessChecker: accessChecker,
	}
}

func (s *service) Get(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*profiledomain.Patient, error) {
	if err := s.accessChecker.RequireAccess(ctx, currentAccountID(currentAccount), id); err != nil {
		return nil, err
	}

	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoError("patientRepo.FindByID", err)
	}
	if p == nil {
		return nil, patientNotFound()
	}
	return p, nil
}

func (s *service) Update(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID, input UpdateInput) (*profiledomain.Patient, error) {
	if err := s.accessChecker.RequireAccess(ctx, currentAccountID(currentAccount), id); err != nil {
		return nil, err
	}

	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoError("patientRepo.FindByID", err)
	}
	if p == nil {
		return nil, patientNotFound()
	}

	p.ApplyUpdate(
		input.FullName,
		input.Phone,
		input.AvatarURL,
		input.Gender,
		input.Race,
		input.CNS,
	)

	if err := p.Validate(); err != nil {
		return nil, mapDomainError(err)
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, mapRepoError("patientRepo.Update", err)
	}
	return p, nil
}

func (s *service) SoftDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error {
	if err := s.requireProfessionalAccess(ctx, currentAccount, id); err != nil {
		return err
	}

	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return mapRepoError("patientRepo.FindByID", err)
	}
	if p == nil {
		return patientNotFound()
	}

	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return mapRepoError("patientRepo.SoftDelete", err)
	}
	return nil
}

func (s *service) HardDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error {
	if err := s.requireProfessionalAccess(ctx, currentAccount, id); err != nil {
		return err
	}

	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return mapRepoError("patientRepo.FindByID", err)
	}
	if p == nil {
		return patientNotFound()
	}

	if err := s.repo.HardDelete(ctx, id); err != nil {
		return mapRepoError("patientRepo.HardDelete", err)
	}
	return nil
}

func (s *service) requireProfessionalAccess(
	ctx context.Context,
	currentAccount *accountdomain.Account,
	patientID uuid.UUID,
) error {
	if currentAccount == nil || currentAccount.ID == uuid.Nil {
		return apperr.Unauthorized("autenticação necessária")
	}
	if currentAccount.DeletedAt != nil || currentAccount.AccountType != accountdomain.AccountTypeProfessional {
		return apperr.Forbidden("acesso negado")
	}
	return s.accessChecker.RequireAccess(ctx, currentAccount.ID, patientID)
}

func currentAccountID(currentAccount *accountdomain.Account) uuid.UUID {
	if currentAccount == nil {
		return uuid.Nil
	}
	return currentAccount.ID
}

func (s *service) ListMyPatients(ctx context.Context, currentAccount *accountdomain.Account, limit, offset int) ([]*profiledomain.Patient, error) {
	if currentAccount == nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}

	if s.accessRepo == nil {
		return nil, apperr.Internal("erro inesperado", errors.New("patient access repository not configured"))
	}

	accessible, _, err := s.accessRepo.ListAccessiblePatientsByUser(ctx, currentAccount.ID, limit, offset)
	if err != nil {
		return nil, &apperr.AppError{
			Kind:    apperr.INFRA_DATABASE_ERROR,
			Message: "falha técnica",
			Cause:   fmt.Errorf("patientAccessRepo.ListAccessiblePatientsByUser: %w", err),
		}
	}

	out := make([]*profiledomain.Patient, 0, len(accessible))
	for _, row := range accessible {
		p, err := s.repo.FindByID(ctx, row.PatientID)
		if err != nil {
			return nil, mapRepoError("patientRepo.FindByID", err)
		}
		if p == nil {
			continue
		}
		out = append(out, p)
	}

	return out, nil
}
