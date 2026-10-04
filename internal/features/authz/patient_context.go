// internal/features/authz/patient_context.go
package authz

import (
	"context"
	"errors"
	"fmt"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accessdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/access/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type accountLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*accountdomain.User, error)
}

type accessChecker interface {
	RequireAccess(ctx context.Context, accountID, patientID uuid.UUID) error
}

type relationshipLookup interface {
	FindActiveRelationship(ctx context.Context, patientID, accountID uuid.UUID) (*accessdomain.RelationshipType, error)
}

type selfIdentityLookup interface {
	HasActiveSelf(ctx context.Context, patientID, accountID uuid.UUID) (bool, error)
}

// PatientContextResolver loads current authorization facts from backend stores.
// Caregiver authorization remains false until A1.4 defines a trusted grant.
type PatientContextResolver struct {
	accounts      accountLookup
	access        accessChecker
	relationships relationshipLookup
	selfIdentity  selfIdentityLookup
}

func NewPatientContextResolver(accounts accountLookup, access accessChecker, relationships relationshipLookup, selfIdentity selfIdentityLookup) *PatientContextResolver {
	return &PatientContextResolver{accounts: accounts, access: access, relationships: relationships, selfIdentity: selfIdentity}
}

func (r *PatientContextResolver) Resolve(ctx context.Context, accountID, patientID uuid.UUID) (PatientContext, error) {
	if accountID == uuid.Nil {
		return PatientContext{}, apperr.Unauthorized("autenticação necessária")
	}
	if patientID == uuid.Nil {
		return PatientContext{}, apperr.Forbidden("acesso negado")
	}
	if r == nil || r.accounts == nil || r.access == nil || r.relationships == nil || r.selfIdentity == nil {
		return PatientContext{}, apperr.Internal("erro inesperado", errors.New("patient context dependencies not configured"))
	}
	account, err := r.accounts.FindByID(ctx, accountID)
	if err != nil {
		return PatientContext{}, contextStoreError("accounts.FindByID", err)
	}
	if account == nil || account.ID != accountID || !account.AccountType.IsValid() {
		return PatientContext{}, apperr.Forbidden("acesso negado")
	}
	if err := r.access.RequireAccess(ctx, accountID, patientID); err != nil {
		return PatientContext{}, err
	}
	relation, err := r.relationships.FindActiveRelationship(ctx, patientID, accountID)
	if err != nil {
		return PatientContext{}, contextStoreError("relationships.FindActiveRelationship", err)
	}
	verified, err := r.selfIdentity.HasActiveSelf(ctx, patientID, accountID)
	if err != nil {
		return PatientContext{}, contextStoreError("selfIdentity.HasActiveSelf", err)
	}
	return PatientContext{
		AccountID:        accountID,
		PatientID:        patientID,
		AccountType:      account.AccountType,
		HasAccess:        true,
		SelfVerified:     verified,
		RelationshipType: relation,
	}, nil
}

func contextStoreError(operation string, err error) error {
	return &apperr.AppError{Kind: apperr.INFRA_DATABASE_ERROR, Message: "falha técnica", Cause: fmt.Errorf("%s: %w", operation, err)}
}
