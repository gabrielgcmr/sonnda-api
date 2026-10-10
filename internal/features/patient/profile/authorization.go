// internal/features/patient/profile/authorization.go
package patientprofile

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// Authorizer is the authorization boundary consumed by the profile service.
// It returns the patient loaded while resolving access so callers do not fetch it again.
type Authorizer interface {
	Authorize(
		ctx context.Context,
		currentAccount *accountdomain.Account,
		patientID uuid.UUID,
		action Action,
	) (*profiledomain.Patient, error)
}

type patientAccessResolver interface {
	ResolveAccessiblePatient(ctx context.Context, accountID, patientID uuid.UUID) (*profiledomain.Patient, error)
}

type authorizer struct {
	patients patientAccessResolver
}

func NewAuthorizer(patients patientAccessResolver) Authorizer {
	return &authorizer{patients: patients}
}

func (a *authorizer) Authorize(
	ctx context.Context,
	currentAccount *accountdomain.Account,
	patientID uuid.UUID,
	action Action,
) (*profiledomain.Patient, error) {
	if a == nil || a.patients == nil {
		return nil, apperr.Internal("erro inesperado", errors.New("profile authorizer dependencies not configured"))
	}

	actor := authz.PatientContext{PatientID: patientID}
	if currentAccount != nil {
		actor.AccountID = currentAccount.ID
		actor.AccountType = currentAccount.AccountType
		if currentAccount.DeletedAt != nil {
			return nil, apperr.Forbidden("acesso negado")
		}
	}
	if err := requireAccountAction(action, actor); err != nil {
		return nil, err
	}

	patient, err := a.patients.ResolveAccessiblePatient(ctx, actor.AccountID, patientID)
	if err != nil {
		return nil, err
	}
	actor.HasAccess = true
	if err := RequireAction(action, patientID, actor); err != nil {
		return nil, err
	}
	return patient, nil
}

var _ Authorizer = (*authorizer)(nil)
