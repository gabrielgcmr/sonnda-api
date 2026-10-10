// internal/features/capture/http/middleware.go
package capturehttp

import (
	"context"
	"errors"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type mobileAuthenticator interface {
	AuthenticateMobile(context.Context, string) (*capture.MobileCredential, error)
}

type Middleware struct {
	authenticator mobileAuthenticator
}

func NewMiddleware(authenticator mobileAuthenticator) *Middleware {
	return &Middleware{authenticator: authenticator}
}

func (m *Middleware) Authenticate(ctx context.Context, uploadToken string) (*capture.MobileCredential, error) {
	if m == nil || m.authenticator == nil {
		return nil, apperr.Internal("autenticação de captura indisponível", errors.New("capture authenticator is not configured"))
	}
	return m.authenticator.AuthenticateMobile(ctx, uploadToken)
}
