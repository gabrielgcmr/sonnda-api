// internal/features/patient/profile/service.go
package patientprofile

import (
	"context"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"

	"github.com/google/uuid"
)

type Service interface {
	Get(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*profiledomain.Patient, error)
	Update(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID, input UpdateInput) (*profiledomain.Patient, error)
	SoftDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error
	HardDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error
}

type service struct {
	repo       Repository
	authorizer Authorizer
}

var _ Service = (*service)(nil)

func New(
	repo Repository,
	authorizer Authorizer,
) Service {
	return &service{
		repo:       repo,
		authorizer: authorizer,
	}
}

func (s *service) Get(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*profiledomain.Patient, error) {
	p, err := s.authorizer.Authorize(ctx, currentAccount, id, ReadProfile)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *service) Update(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID, input UpdateInput) (*profiledomain.Patient, error) {
	p, err := s.authorizer.Authorize(ctx, currentAccount, id, UpdateProfile)
	if err != nil {
		return nil, err
	}

	if err := p.ApplyUpdate(
		input.FullName,
		input.Phone,
		input.AvatarURL,
		input.Gender,
		input.Race,
		input.CNS,
	); err != nil {
		return nil, mapDomainError(err)
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, mapRepoError("patientRepo.Update", err)
	}
	return p, nil
}

func (s *service) SoftDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error {
	if _, err := s.authorizer.Authorize(ctx, currentAccount, id, SoftDeleteProfile); err != nil {
		return err
	}

	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return mapRepoError("patientRepo.SoftDelete", err)
	}
	return nil
}

func (s *service) HardDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error {
	if _, err := s.authorizer.Authorize(ctx, currentAccount, id, HardDeleteProfile); err != nil {
		return err
	}

	if err := s.repo.HardDelete(ctx, id); err != nil {
		return mapRepoError("patientRepo.HardDelete", err)
	}
	return nil
}
