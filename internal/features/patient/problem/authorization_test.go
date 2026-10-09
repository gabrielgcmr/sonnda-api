// internal/features/patient/problem/authorization_test.go
package problem

import (
	"context"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type authorizationContextResolver struct {
	actor authz.PatientContext
	err   error
}

func (r authorizationContextResolver) Resolve(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (authz.PatientContext, error) {
	return r.actor, r.err
}

func TestAuthorizerCombinesSharedContextAndProblemPolicy(t *testing.T) {
	patientID := uuid.New()
	basicCareID := uuid.New()
	basicCare := NewAuthorizer(authorizationContextResolver{actor: authz.PatientContext{
		AccountID: basicCareID, PatientID: patientID,
		AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true,
	}})

	if err := basicCare.Authorize(context.Background(), basicCareID, patientID, ResolveProblem); err != nil {
		t.Fatalf("basic care with access should resolve: %v", err)
	}
	if err := basicCare.Authorize(context.Background(), basicCareID, patientID, CreateProblem); errorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("basic care should not create: %v", err)
	}

	professionalID := uuid.New()
	professional := NewAuthorizer(authorizationContextResolver{actor: authz.PatientContext{
		AccountID: professionalID, PatientID: patientID,
		AccountType: accountdomain.AccountTypeProfessional, HasAccess: true,
	}})
	if err := professional.Authorize(context.Background(), professionalID, patientID, CreateProblem); err != nil {
		t.Fatalf("professional with access should create: %v", err)
	}
}

func TestAuthorizerPropagatesSharedContextFailure(t *testing.T) {
	denied := apperr.Forbidden("acesso negado")
	authorizer := NewAuthorizer(authorizationContextResolver{err: denied})

	if err := authorizer.Authorize(context.Background(), uuid.New(), uuid.New(), ResolveProblem); err != denied {
		t.Fatalf("expected context failure to be preserved: %v", err)
	}
}

func TestAuthorizerRequiresConfiguredContext(t *testing.T) {
	authorizer := NewAuthorizer(nil)
	if err := authorizer.Authorize(context.Background(), uuid.New(), uuid.New(), ReadProblem); errorKind(err) != apperr.INTERNAL_ERROR {
		t.Fatalf("expected internal error: %v", err)
	}
}
