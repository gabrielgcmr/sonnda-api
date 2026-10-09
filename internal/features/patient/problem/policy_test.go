// internal/features/patient/problem/policy_test.go
package problem

import (
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

func TestPolicy(t *testing.T) {
	patientID := uuid.New()
	professional := authz.PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeProfessional, HasAccess: true}
	basicCare := authz.PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true}

	for _, action := range []Action{ListProblems, ReadProblem, ReadHistory, ResolveProblem} {
		assertAction(t, "professional", action, patientID, professional, true)
		assertAction(t, "basic care", action, patientID, basicCare, true)
	}
	for _, action := range []Action{CreateProblem, EditProblem, ClassifyProblem, ReopenProblem, RectifyProblem, MergeProblems} {
		assertAction(t, "professional", action, patientID, professional, true)
		assertAction(t, "basic care", action, patientID, basicCare, false)
	}
}

func TestPolicyResolutionDependsOnActiveAccess(t *testing.T) {
	patientID := uuid.New()
	actor := authz.PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true}

	assertAction(t, "active access", ResolveProblem, patientID, actor, true)
	actor.HasAccess = false
	assertAction(t, "no access", ResolveProblem, patientID, actor, false)
	actor.HasAccess = true
	assertAction(t, "another patient", ResolveProblem, uuid.New(), actor, false)
}

func TestPolicyRequiresAccessToThisPatient(t *testing.T) {
	patientID := uuid.New()
	actor := authz.PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeProfessional, HasAccess: true}
	assertAction(t, "other patient", CreateProblem, uuid.New(), actor, false)
	assertAction(t, "no access", ListProblems, patientID, authz.PatientContext{AccountID: actor.AccountID, PatientID: patientID, AccountType: actor.AccountType}, false)
	assertAction(t, "invalid action", Action("unknown"), patientID, actor, false)
	assertAction(t, "invalid account type", ListProblems, patientID, authz.PatientContext{AccountID: actor.AccountID, PatientID: patientID, HasAccess: true}, false)
	assertAction(t, "missing patient", ListProblems, uuid.Nil, actor, false)
}

func TestPolicyRequiresAccount(t *testing.T) {
	patientID := uuid.New()
	err := RequireAction(ListProblems, patientID, authz.PatientContext{PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true})
	if errorKind(err) != apperr.AUTH_REQUIRED {
		t.Fatalf("expected authentication error, got %v", err)
	}
}

func assertAction(t *testing.T, name string, action Action, patientID uuid.UUID, actor authz.PatientContext, allowed bool) {
	t.Helper()
	err := RequireAction(action, patientID, actor)
	if allowed && err != nil {
		t.Fatalf("%s: %s should be allowed: %v", name, action, err)
	}
	if !allowed && errorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("%s: %s should be forbidden, got %v", name, action, err)
	}
}

func errorKind(err error) apperr.ErrorKind {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return ""
}
