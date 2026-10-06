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
		contextAccounts{user: &accountdomain.Account{ID: basicCareID, AccountType: accountdomain.AccountTypeBasicCare}},
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
		contextAccounts{user: &accountdomain.Account{ID: professionalID, AccountType: accountdomain.AccountTypeProfessional}},
		contextAccess{},
	))
	if err := professional.Authorize(context.Background(), professionalID, patientID, CreateProblem); err != nil {
		t.Fatalf("professional with access should create: %v", err)
	}
}

func TestProblemAuthorizerPropagatesContextFailure(t *testing.T) {
	accountID, patientID := uuid.New(), uuid.New()
	authorizer := NewProblemAuthorizer(NewPatientContextResolver(
		contextAccounts{user: &accountdomain.Account{ID: accountID, AccountType: accountdomain.AccountTypeBasicCare}},
		contextAccess{err: apperr.Forbidden("acesso negado")},
	))

	if err := authorizer.Authorize(context.Background(), accountID, patientID, ResolveProblem); contextErrorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("expected access denial: %v", err)
	}
}

func TestProblemAuthorizerRejectsUnauthorizedRequests(t *testing.T) {
	patientID := uuid.New()
	professionalID := uuid.New()
	professional := contextAccounts{user: &accountdomain.Account{
		ID:          professionalID,
		AccountType: accountdomain.AccountTypeProfessional,
	}}

	for _, tc := range []struct {
		name      string
		accountID uuid.UUID
		action    ProblemAction
		accessErr error
		wantKind  apperr.ErrorKind
	}{
		{
			name:      "professional without access",
			accountID: professionalID,
			action:    CreateProblem,
			accessErr: apperr.Forbidden("acesso negado"),
			wantKind:  apperr.ACCESS_DENIED,
		},
		{
			name:      "unknown action",
			accountID: professionalID,
			action:    ProblemAction("unknown"),
			wantKind:  apperr.ACCESS_DENIED,
		},
		{
			name:     "missing identity",
			action:   ReadProblem,
			wantKind: apperr.AUTH_REQUIRED,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authorizer := NewProblemAuthorizer(NewPatientContextResolver(
				professional,
				contextAccess{err: tc.accessErr},
			))
			err := authorizer.Authorize(t.Context(), tc.accountID, patientID, tc.action)
			if contextErrorKind(err) != tc.wantKind {
				t.Fatalf("expected %s, got %v", tc.wantKind, err)
			}
		})
	}
}

func TestProblemAuthorizerRequiresConfiguredContext(t *testing.T) {
	authorizer := NewProblemAuthorizer(nil)
	if err := authorizer.Authorize(context.Background(), uuid.New(), uuid.New(), ReadProblem); contextErrorKind(err) != apperr.INTERNAL_ERROR {
		t.Fatalf("expected internal error: %v", err)
	}
}
