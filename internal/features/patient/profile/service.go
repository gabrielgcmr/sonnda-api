// internal/features/patient/profile/service.go
package patientprofile

import (
	"context"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"

	"github.com/google/uuid"
)

type Service interface {
	Get(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*profiledomain.Patient, error)
	Update(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID, input UpdateInput) (*profiledomain.Patient, error)
	SoftDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error
	HardDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error
}

type AccessResolver interface {
	ResolveAccessiblePatient(ctx context.Context, accountID, patientID uuid.UUID) (*profiledomain.Patient, error)
}

type service struct {
	repo           Repository
	accessResolver AccessResolver
}

var _ Service = (*service)(nil)

func New(
	repo Repository,
	accessResolver AccessResolver,
) Service {
	return &service{
		repo:           repo,
		accessResolver: accessResolver,
	}
}

func (s *service) Get(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*profiledomain.Patient, error) {
	p, err := s.accessResolver.ResolveAccessiblePatient(ctx, currentAccountID(currentAccount), id)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *service) Update(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID, input UpdateInput) (*profiledomain.Patient, error) {
	p, err := s.accessResolver.ResolveAccessiblePatient(ctx, currentAccountID(currentAccount), id)
	if err != nil {
		return nil, err
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
	if _, err := s.resolveProfessionalAccess(ctx, currentAccount, id); err != nil {
		return err
	}

	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return mapRepoError("patientRepo.SoftDelete", err)
	}
	return nil
}

func (s *service) HardDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error {
	if _, err := s.resolveProfessionalAccess(ctx, currentAccount, id); err != nil {
		return err
	}

	if err := s.repo.HardDelete(ctx, id); err != nil {
		return mapRepoError("patientRepo.HardDelete", err)
	}
	return nil
}

func (s *service) resolveProfessionalAccess(
	ctx context.Context,
	currentAccount *accountdomain.Account,
	patientID uuid.UUID,
) (*profiledomain.Patient, error) {
	if currentAccount == nil || currentAccount.ID == uuid.Nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}
	if currentAccount.DeletedAt != nil || currentAccount.AccountType != accountdomain.AccountTypeProfessional {
		return nil, apperr.Forbidden("acesso negado")
	}
	return s.accessResolver.ResolveAccessiblePatient(ctx, currentAccount.ID, patientID)
}

func currentAccountID(currentAccount *accountdomain.Account) uuid.UUID {
	if currentAccount == nil {
		return uuid.Nil
	}
	return currentAccount.ID
}
