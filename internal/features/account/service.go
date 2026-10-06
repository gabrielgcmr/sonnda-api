// internal/features/account/service.go
package account

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"

	"github.com/google/uuid"
)

type Service interface {
	Create(ctx context.Context, input AccountCreateInput) (*accountdomain.Account, error)
	Update(ctx context.Context, input AccountUpdateInput) (*accountdomain.Account, error)
	SoftDelete(ctx context.Context, userID uuid.UUID) error
}
type service struct {
	accountRepo Repository
}

var _ Service = (*service)(nil)

func New(accountRepo Repository) Service {
	return &service{accountRepo: accountRepo}
}

func (s *service) Create(ctx context.Context, input AccountCreateInput) (*accountdomain.Account, error) {
	newAccount, err := accountdomain.NewAccount(accountdomain.NewAccountParams{
		AccountType: input.AccountType,
		Profile:     input.Profile,
	})
	if err != nil {
		return nil, mapDomainError(err)
	}

	identity, err := accountdomain.NewIdentity(newAccount.ID, input.Issuer, input.Subject, input.Email)
	if err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.accountRepo.Create(ctx, newAccount, identity); err != nil {
		return nil, mapRepoError("accountRepo.Create", err)
	}

	return newAccount, nil
}

func (s *service) Update(ctx context.Context, input AccountUpdateInput) (*accountdomain.Account, error) {
	existingUser, err := s.accountRepo.FindByID(ctx, input.AccountID)
	if err != nil {
		return nil, mapRepoError("accountRepo.FindByID", err)
	}
	if existingUser == nil {
		return nil, accountNotFound()
	}

	if existingUser.DeletedAt != nil {
		return nil, apperr.Forbidden("conta desativada")
	}
	profile := existingUser.Profile
	if input.FullName != nil {
		profile.FullName = input.FullName
	}
	if input.BirthDate != nil {
		profile.BirthDate = input.BirthDate
	}
	if input.CPF != nil {
		profile.CPF = input.CPF
	}
	if input.Phone != nil {
		profile.Phone = input.Phone
	}
	changed, err := existingUser.ApplyProfile(profile)
	if err != nil {
		return nil, mapDomainError(err)
	}
	if !changed {
		return existingUser, nil
	}

	if err := s.accountRepo.Update(ctx, existingUser); err != nil {
		return nil, mapRepoError("accountRepo.Update", err)
	}

	return existingUser, nil
}

func (s *service) SoftDelete(ctx context.Context, userID uuid.UUID) error {
	// NOTE: SoftDelete is intentionally idempotent.
	//
	// We first load the user to return a proper NOT_FOUND when it truly doesn't exist.
	// Then we execute the delete; if the repository reports NOT_FOUND at this stage
	// (e.g. already deleted or a race where another request deleted it), we treat it as success.
	existing, err := s.accountRepo.FindByID(ctx, userID)
	if err != nil {
		return mapRepoError("accountRepo.FindByID", err)
	}
	if existing == nil {
		return accountNotFound()
	}

	if err := s.accountRepo.SoftDelete(ctx, userID); err != nil {
		mapped := mapRepoError("accountRepo.SoftDelete", err)
		var appErr *apperr.AppError
		if errors.As(mapped, &appErr) && appErr != nil && appErr.Kind == apperr.NOT_FOUND {
			return nil
		}
		return mapped
	}

	return nil
}
