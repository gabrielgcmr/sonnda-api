// internal/features/authz/problem_authorizer_test.go
package authz

import (
	"context"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

func TestProblemAuthorizerCombinesContextAndPolicy(t *testing.T) {
	patientID := uuid.New()
	basicCareID := uuid.New()
	basicCare := NewProblemAuthorizer(NewPatientContextResolver(
		contextAccounts{user: &accountdomain.User{ID: basicCareID, AccountType: accountdomain.AccountTypeBasicCare}},
		contextAccess{},
	))

	if err := basicCare.Authorize(context.Background(), basicCareID, patientID, ResolveProblem); err != nil {
		t.Fatalf("basic care with access should resolve: %v", err)
	}
	if err := basicCare.Authorize(context.Background(), basicCareID, patientID, CreateProblem); contextErrorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("basic care should not create: %v", err)
	}

	professionalID := uuid.New()
	professional := NewProblemAuthorizer(NewPatientContextResolver(
		contextAccounts{user: &accountdomain.User{ID: professionalID, AccountType: accountdomain.AccountTypeProfessional}},
		contextAccess{},
	))
	if err := professional.Authorize(context.Background(), professionalID, patientID, CreateProblem); err != nil {
		t.Fatalf("professional with access should create: %v", err)
	}
}

func TestProblemAuthorizerPropagatesContextFailure(t *testing.T) {
	accountID, patientID := uuid.New(), uuid.New()
	authorizer := NewProblemAuthorizer(NewPatientContextResolver(
		contextAccounts{user: &accountdomain.User{ID: accountID, AccountType: accountdomain.AccountTypeBasicCare}},
		contextAccess{err: apperr.Forbidden("acesso negado")},
	))

	if err := authorizer.Authorize(context.Background(), accountID, patientID, ResolveProblem); contextErrorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("expected access denial: %v", err)
	}
}

func TestProblemAuthorizerRequiresConfiguredContext(t *testing.T) {
	authorizer := NewProblemAuthorizer(nil)
	if err := authorizer.Authorize(context.Background(), uuid.New(), uuid.New(), ReadProblem); contextErrorKind(err) != apperr.INTERNAL_ERROR {
		t.Fatalf("expected internal error: %v", err)
	}
}
