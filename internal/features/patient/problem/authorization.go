// internal/features/patient/problem/authorization.go
package problem

import (
	"context"
	"errors"

	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// Authorizer is the authorization boundary consumed by the problem service.
// It combines shared account/patient facts with the policy owned by this feature.
type Authorizer interface {
	Authorize(ctx context.Context, accountID, patientID uuid.UUID, action Action) error
}

type patientContextResolver interface {
	Resolve(ctx context.Context, accountID, patientID uuid.UUID) (authz.PatientContext, error)
}

type authorizer struct {
	contexts patientContextResolver
}

func NewAuthorizer(contexts patientContextResolver) Authorizer {
	return &authorizer{contexts: contexts}
}

func (a *authorizer) Authorize(ctx context.Context, accountID, patientID uuid.UUID, action Action) error {
	if a == nil || a.contexts == nil {
		return apperr.Internal("erro inesperado", errors.New("problem authorizer dependencies not configured"))
	}
	actor, err := a.contexts.Resolve(ctx, accountID, patientID)
	if err != nil {
		return err
	}
	return RequireAction(action, patientID, actor)
}

var _ Authorizer = (*authorizer)(nil)
