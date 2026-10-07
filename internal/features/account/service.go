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
	ResolveOrProvision(ctx context.Context, input AccountResolveInput) (*accountdomain.Account, error)
	Update(ctx context.Context, input AccountUpdateInput) (*accountdomain.Account, error)
	DeactivateByIdentity(ctx context.Context, issuer, subject string) error
}

func (s *service) ResolveOrProvision(ctx context.Context, input AccountResolveInput) (*accountdomain.Account, error) {
	candidate, err := accountdomain.NewAccount(accountdomain.NewAccountParams{
		AccountType: accountdomain.AccountTypeBasicCare,
	})
	if err != nil {
		return nil, mapDomainError(err)
	}
	identity, err := accountdomain.NewIdentity(candidate.ID, input.Issuer, input.Subject, input.Email)
	if err != nil {
		return nil, mapDomainError(err)
	}

	var resolved *accountdomain.Account
	err = s.accountRepo.WithinTransaction(ctx, func(txRepo Repository) error {
		if err := txRepo.LockAuthIdentity(ctx, identity.Issuer, identity.Subject); err != nil {
			return err
		}

		existing, err := txRepo.FindByAuthIdentity(ctx, identity.Issuer, identity.Subject)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.DeletedAt != nil {
				return apperr.AccountDeactivated()
			}
			if err := txRepo.UpdateIdentityEmail(ctx, identity.Issuer, identity.Subject, identity.Email); err != nil {
				return err
			}
			resolved = existing
			return nil
		}

		if err := txRepo.Create(ctx, candidate, identity); err != nil {
			return err
		}
		resolved = candidate
		return nil
	})
	if err != nil {
		return nil, mapRepoError("accountRepo.ResolveOrProvision", err)
	}
	return resolved, nil
}

type service struct {
	accountRepo Repository
}

var _ Service = (*service)(nil)

func New(accountRepo Repository) Service {
	return &service{accountRepo: accountRepo}
}

func (s *service) Update(ctx context.Context, input AccountUpdateInput) (*accountdomain.Account, error) {
	var updated *accountdomain.Account
	err := s.accountRepo.WithinTransaction(ctx, func(txRepo Repository) error {
		existing, err := txRepo.FindByIDForUpdate(ctx, input.AccountID)
		if err != nil {
			return err
		}
		if existing == nil {
			return accountNotFound()
		}
		if existing.DeletedAt != nil {
			return apperr.AccountDeactivated()
		}

		profile := existing.Profile
		if input.FullName.Set {
			profile.FullName = input.FullName.Value
		}
		if input.BirthDate.Set {
			profile.BirthDate = input.BirthDate.Value
		}
		if input.CPF.Set {
			profile.CPF = input.CPF.Value
		}
		if input.Phone.Set {
			profile.Phone = input.Phone.Value
		}

		next := *existing
		changed, err := next.ApplyProfile(profile)
		if err != nil {
			return mapDomainError(err)
		}
		if changed {
			if err := txRepo.Update(ctx, &next); err != nil {
				return err
			}
		}
		updated = &next
		return nil
	})
	if err != nil {
		return nil, mapRepoError("accountRepo.Update", err)
	}
	return updated, nil
}

func (s *service) DeactivateByIdentity(ctx context.Context, issuer, subject string) error {
	if _, err := accountdomain.NewIdentity(uuid.New(), issuer, subject, nil); err != nil {
		return mapDomainError(err)
	}

	err := s.accountRepo.WithinTransaction(ctx, func(txRepo Repository) error {
		if err := txRepo.LockAuthIdentity(ctx, issuer, subject); err != nil {
			return err
		}
		existing, err := txRepo.FindByAuthIdentity(ctx, issuer, subject)
		if err != nil {
			return err
		}
		if existing == nil {
			return accountNotFound()
		}
		if existing.DeletedAt != nil {
			return nil
		}
		if err := txRepo.SoftDelete(ctx, existing.ID); err != nil {
			if errors.Is(err, ErrAccountNotFound) {
				return nil
			}
			return err
		}
		return nil
	})
	if err != nil {
		return mapRepoError("accountRepo.DeactivateByIdentity", err)
	}
	return nil
}
