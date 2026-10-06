// internal/features/account/onboarding.go
package account

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type Onboarding interface {
	Register(ctx context.Context, input RegisterInput) (*accountdomain.Account, error)
}

type onboarding struct {
	accountRepo Repository
	accountSvc  Service
}

var _ Onboarding = (*onboarding)(nil)

func NewOnboarding(accountRepo Repository, accountSvc Service) Onboarding {
	return &onboarding{
		accountRepo: accountRepo,
		accountSvc:  accountSvc,
	}
}

func (u *onboarding) Register(ctx context.Context, input RegisterInput) (*accountdomain.Account, error) {
	// Verificar se usuário já existe
	existing, err := u.accountRepo.FindByAuthIdentity(ctx, input.Issuer, input.Subject)
	if err != nil {
		return nil, apperr.Internal("falha ao verificar registro", err)
	}
	if existing != nil {
		return nil, apperr.AlreadyExists("usuário já cadastrado")
	}

	createdUser, err := u.accountSvc.Create(ctx, AccountCreateInput{
		Issuer:      input.Issuer,
		Subject:     input.Subject,
		Email:       input.Email,
		AccountType: input.AccountType,
		Profile:     input.Profile,
	})
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) && appErr != nil {
			return nil, appErr
		}
		return nil, apperr.Internal("falha ao criar usuário", err)
	}

	return createdUser, nil
}
