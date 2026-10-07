// internal/api/middleware/auth.go
package middleware

import (
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

// RequireBearer authenticates the request before running a Huma operation.
func RequireBearer(api huma.API, auth *authhttp.Middleware) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		identity, err := auth.AuthenticateBearer(ctx.Context(), ctx.Header("Authorization"))
		if err != nil {
			_ = humaerror.Write(api, ctx, err)
			return
		}

		ginContext := humagin.Unwrap(ctx)
		ginContext.Request = ginContext.Request.WithContext(authhttp.ContextWithIdentity(ctx.Context(), identity))
		next(ctx)
	}
}

// ResolveAccount resolves or provisions the application account for an authenticated request.
func ResolveAccount(api huma.API, account *accounthttp.Middleware) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		identity, ok := authhttp.GetIdentityFromContext(ctx.Context())
		if !ok {
			_ = humaerror.Write(api, ctx, apperr.Unauthorized("autenticação necessária"))
			return
		}

		currentAccount, err := account.ResolveAccount(ctx.Context(), identity)
		if err != nil {
			_ = humaerror.Write(api, ctx, err)
			return
		}

		if currentAccount == nil {
			_ = humaerror.Write(api, ctx, apperr.Internal("falha ao resolver conta", errors.New("account resolver returned nil without an error")))
			return
		}

		ginContext := humagin.Unwrap(ctx)
		ginContext.Request = ginContext.Request.WithContext(helpers.ContextWithCurrentAccount(ctx.Context(), currentAccount))
		next(ctx)
	}
}

// RequireCompletedOnboarding blocks business operations until the minimum
// account profile (valid name and birth date) has been provided.
func RequireCompletedOnboarding(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx.Context())
		if !ok {
			_ = humaerror.Write(api, ctx, apperr.Internal("falha ao resolver conta", errors.New("current account is missing before onboarding middleware")))
			return
		}
		if !currentAccount.OnboardingCompleted() {
			_ = humaerror.Write(api, ctx, apperr.OnboardingRequired())
			return
		}
		next(ctx)
	}
}
