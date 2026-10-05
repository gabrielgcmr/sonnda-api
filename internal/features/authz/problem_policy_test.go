// internal/features/authz/problem_policy_test.go
package authz

import (
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

func TestProblemPolicy(t *testing.T) {
	patientID := uuid.New()
	professional := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeProfessional, HasAccess: true}
	basicCare := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true}

	for _, action := range []ProblemAction{ListProblems, ReadProblem, ReadHistory, ResolveProblem} {
		assertProblemAction(t, "professional", action, patientID, professional, true)
		assertProblemAction(t, "basic care", action, patientID, basicCare, true)
	}
	for _, action := range []ProblemAction{CreateProblem, EditProblem, ClassifyProblem, ReopenProblem, RectifyProblem, MergeProblems} {
		assertProblemAction(t, "professional", action, patientID, professional, true)
		assertProblemAction(t, "basic care", action, patientID, basicCare, false)
	}
}

func TestProblemPolicyResolutionDependsOnActiveAccess(t *testing.T) {
	patientID := uuid.New()
	actor := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true}

	assertProblemAction(t, "active access", ResolveProblem, patientID, actor, true)
	actor.HasAccess = false
	assertProblemAction(t, "no access", ResolveProblem, patientID, actor, false)
	actor.HasAccess = true
	assertProblemAction(t, "another patient", ResolveProblem, uuid.New(), actor, false)
}

func TestProblemPolicyRequiresAccessToThisPatient(t *testing.T) {
	patientID := uuid.New()
	actor := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeProfessional, HasAccess: true}
	assertProblemAction(t, "other patient", CreateProblem, uuid.New(), actor, false)
	assertProblemAction(t, "no access", ListProblems, patientID, PatientContext{AccountID: actor.AccountID, PatientID: patientID, AccountType: actor.AccountType}, false)
	assertProblemAction(t, "invalid action", ProblemAction("unknown"), patientID, actor, false)
	assertProblemAction(t, "invalid account type", ListProblems, patientID, PatientContext{AccountID: actor.AccountID, PatientID: patientID, HasAccess: true}, false)
	assertProblemAction(t, "missing patient", ListProblems, uuid.Nil, actor, false)
}

func TestProblemPolicyRequiresAccount(t *testing.T) {
	patientID := uuid.New()
	err := RequireProblemAction(ListProblems, patientID, PatientContext{PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Kind != apperr.AUTH_REQUIRED {
		t.Fatalf("expected authentication error, got %v", err)
	}
}

func assertProblemAction(t *testing.T, name string, action ProblemAction, patientID uuid.UUID, actor PatientContext, allowed bool) {
	t.Helper()
	err := RequireProblemAction(action, patientID, actor)
	if allowed && err != nil {
		t.Fatalf("%s: %s should be allowed: %v", name, action, err)
	}
	if !allowed {
		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.ACCESS_DENIED {
			t.Fatalf("%s: %s should be forbidden, got %v", name, action, err)
		}
	}
}
