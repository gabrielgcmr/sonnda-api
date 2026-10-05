// internal/features/authz/problem_authorizer.go
package authz

import (
	"context"
	"errors"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// ProblemAuthorizer is the authorization boundary consumed by patient/problem.
// It validates account and patient access before applying the requested policy.
type ProblemAuthorizer interface {
	Authorize(ctx context.Context, accountID, patientID uuid.UUID, action ProblemAction) error
}

type problemAuthorizer struct {
	contexts *PatientContextResolver
}

func NewProblemAuthorizer(contexts *PatientContextResolver) ProblemAuthorizer {
	return &problemAuthorizer{contexts: contexts}
}

func (a *problemAuthorizer) Authorize(ctx context.Context, accountID, patientID uuid.UUID, action ProblemAction) error {
	if a == nil || a.contexts == nil {
		return apperr.Internal("erro inesperado", errors.New("problem authorizer dependencies not configured"))
	}
	actor, err := a.contexts.Resolve(ctx, accountID, patientID)
	if err != nil {
		return err
	}
	return RequireProblemAction(action, patientID, actor)
}

var _ ProblemAuthorizer = (*problemAuthorizer)(nil)
