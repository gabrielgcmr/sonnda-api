// internal/features/authz/patient_context.go
package authz

import (
	"context"
	"errors"
	"fmt"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type accountLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*accountdomain.Account, error)
}

type accessChecker interface {
	RequireAccess(ctx context.Context, accountID, patientID uuid.UUID) error
}

// PatientContext contains trusted account and patient-access facts shared by
// feature-owned authorization policies.
type PatientContext struct {
	AccountID   uuid.UUID
	PatientID   uuid.UUID
	AccountType accountdomain.AccountType
	HasAccess   bool
}

// PatientContextResolver loads current authorization facts from backend stores.
type PatientContextResolver struct {
	accounts accountLookup
	access   accessChecker
}

func NewPatientContextResolver(accounts accountLookup, access accessChecker) *PatientContextResolver {
	return &PatientContextResolver{accounts: accounts, access: access}
}

func (r *PatientContextResolver) Resolve(ctx context.Context, accountID, patientID uuid.UUID) (PatientContext, error) {
	if accountID == uuid.Nil {
		return PatientContext{}, apperr.Unauthorized("autenticação necessária")
	}
	if patientID == uuid.Nil {
		return PatientContext{}, apperr.Forbidden("acesso negado")
	}
	if r == nil || r.accounts == nil || r.access == nil {
		return PatientContext{}, apperr.Internal("erro inesperado", errors.New("patient context dependencies not configured"))
	}
	account, err := r.accounts.FindByID(ctx, accountID)
	if err != nil {
		return PatientContext{}, contextStoreError("accounts.FindByID", err)
	}
	if account == nil || account.ID != accountID || account.DeletedAt != nil || !account.AccountType.IsValid() {
		return PatientContext{}, apperr.Forbidden("acesso negado")
	}
	if err := r.access.RequireAccess(ctx, accountID, patientID); err != nil {
		return PatientContext{}, err
	}
	return PatientContext{
		AccountID:   accountID,
		PatientID:   patientID,
		AccountType: account.AccountType,
		HasAccess:   true,
	}, nil
}

func contextStoreError(operation string, err error) error {
	return &apperr.AppError{Kind: apperr.INFRA_DATABASE_ERROR, Message: "falha técnica", Cause: fmt.Errorf("%s: %w", operation, err)}
}
