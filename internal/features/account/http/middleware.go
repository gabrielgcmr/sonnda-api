// internal/features/account/http/middleware.go
package accounthttp

import (
	"context"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	authdomain "github.com/gabrielgcmr/sonnda/internal/features/auth/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type Middleware struct {
	accountRepo account.Repository
}

func NewMiddleware(accountRepo account.Repository) *Middleware {
	return &Middleware{accountRepo: accountRepo}
}

func (m *Middleware) ResolveRegisteredAccount(ctx context.Context, identity *authdomain.Identity) (*accountdomain.Account, error) {
	if identity == nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}

	currentAccount, err := m.accountRepo.FindByAuthIdentity(ctx, identity.Issuer, identity.Subject)
	if err != nil {
		return nil, apperr.Internal("falha ao buscar usuário", err)
	}

	if currentAccount != nil && currentAccount.DeletedAt != nil {
		return nil, apperr.Forbidden("conta desativada")
	}
	return currentAccount, nil
}
