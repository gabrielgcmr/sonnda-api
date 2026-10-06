// internal/api/helpers/current_account.go
package helpers

import (
	"context"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
)

type currentAccountContextKey struct{}

func ContextWithCurrentAccount(ctx context.Context, user *accountdomain.Account) context.Context {
	return context.WithValue(ctx, currentAccountContextKey{}, user)
}

// GetCurrentAccountFromContext makes the authenticated account available to HTTP
// adapters, such as Huma, that expose only the standard request context.
func GetCurrentAccountFromContext(ctx context.Context) (*accountdomain.Account, bool) {
	u, ok := ctx.Value(currentAccountContextKey{}).(*accountdomain.Account)
	return u, ok && u != nil
}
