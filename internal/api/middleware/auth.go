// internal/api/middleware/auth.go
package middleware

import (
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

// RequireRegisteredAccount resolves the application account for an authenticated request.
func RequireRegisteredAccount(api huma.API, account *accounthttp.Middleware) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		identity, ok := authhttp.GetIdentityFromContext(ctx.Context())
		if !ok {
			_ = humaerror.Write(api, ctx, apperr.Unauthorized("autenticação necessária"))
			return
		}

		currentAccount, err := account.ResolveRegisteredAccount(ctx.Context(), identity)
		if err != nil {
			_ = humaerror.Write(api, ctx, err)
			return
		}

		if currentAccount == nil {
			_ = humaerror.Write(api, ctx, apperr.ProfileNotFound())
			return
		}

		ginContext := humagin.Unwrap(ctx)
		ginContext.Request = ginContext.Request.WithContext(helpers.ContextWithCurrentAccount(ctx.Context(), currentAccount))
		next(ctx)
	}
}
