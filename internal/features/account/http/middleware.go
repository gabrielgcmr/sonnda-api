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
	accountService accountResolver
}

type accountResolver interface {
	ResolveOrProvision(ctx context.Context, input account.AccountResolveInput) (*accountdomain.Account, error)
}

func NewMiddleware(accountService accountResolver) *Middleware {
	return &Middleware{accountService: accountService}
}

func (m *Middleware) ResolveRegisteredAccount(ctx context.Context, identity *authdomain.Identity) (*accountdomain.Account, error) {
	if identity == nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}

	return m.accountService.ResolveOrProvision(ctx, account.AccountResolveInput{
		Issuer:  identity.Issuer,
		Subject: identity.Subject,
		Email:   identity.Email,
	})
}
