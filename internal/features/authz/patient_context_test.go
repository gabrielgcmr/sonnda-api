// internal/features/authz/patient_context_test.go
package authz

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type contextAccounts struct {
	user *accountdomain.Account
	err  error
}

func (a contextAccounts) FindByID(context.Context, uuid.UUID) (*accountdomain.Account, error) {
	return a.user, a.err
}

type contextAccess struct{ err error }

func (a contextAccess) RequireAccess(context.Context, uuid.UUID, uuid.UUID) error { return a.err }

func TestPatientContextUsesRegisteredAccountAndActiveAccess(t *testing.T) {
	accountID, patientID := uuid.New(), uuid.New()
	resolver := NewPatientContextResolver(
		contextAccounts{user: &accountdomain.Account{ID: accountID, AccountType: accountdomain.AccountTypeBasicCare}},
		contextAccess{},
	)

	actor, err := resolver.Resolve(context.Background(), accountID, patientID)
	if err != nil {
		t.Fatal(err)
	}
	if actor.AccountID != accountID || actor.PatientID != patientID || actor.AccountType != accountdomain.AccountTypeBasicCare || !actor.HasAccess {
		t.Fatalf("unexpected context: %+v", actor)
	}
}

func TestPatientContextFailsClosed(t *testing.T) {
	accountID, patientID := uuid.New(), uuid.New()
	accounts := contextAccounts{user: &accountdomain.Account{ID: accountID, AccountType: accountdomain.AccountTypeBasicCare}}
	resolver := NewPatientContextResolver(accounts, contextAccess{err: apperr.Forbidden("acesso negado")})
	if _, err := resolver.Resolve(context.Background(), accountID, patientID); contextErrorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("expected access denial: %v", err)
	}
	resolver.access = contextAccess{}
	resolver.accounts = contextAccounts{err: errors.New("database unavailable")}
	if _, err := resolver.Resolve(context.Background(), accountID, patientID); contextErrorKind(err) != apperr.INFRA_DATABASE_ERROR {
		t.Fatalf("expected database error: %v", err)
	}
	resolver.accounts = contextAccounts{}
	if _, err := resolver.Resolve(context.Background(), accountID, patientID); contextErrorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("expected missing account denial: %v", err)
	}
}

func contextErrorKind(err error) apperr.ErrorKind {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return ""
}
