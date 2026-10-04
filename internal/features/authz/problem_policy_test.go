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
	self := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true, SelfVerified: true}
	caregiver := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true, CaregiverAuthorized: true}
	other := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true}

	readActions := []ProblemAction{ListProblems, ReadProblem, ReadHistory}
	professionalActions := []ProblemAction{CreateProblem, EditProblem, ClassifyProblem, ReopenProblem, RectifyProblem, MergeProblems, GrantCaregiver, RevokeCaregiver}
	for name, actor := range map[string]PatientContext{"professional": professional, "self": self, "caregiver": caregiver, "other": other} {
		for _, action := range readActions {
			assertProblemAction(t, name, action, patientID, actor, true)
		}
	}
	for _, action := range professionalActions {
		assertProblemAction(t, "professional", action, patientID, professional, true)
		assertProblemAction(t, "self", action, patientID, self, false)
		assertProblemAction(t, "caregiver", action, patientID, caregiver, false)
		assertProblemAction(t, "other", action, patientID, other, false)
	}
	assertProblemAction(t, "professional", ResolveProblem, patientID, professional, true)
	assertProblemAction(t, "self", ResolveProblem, patientID, self, true)
	assertProblemAction(t, "caregiver", ResolveProblem, patientID, caregiver, true)
	assertProblemAction(t, "other", ResolveProblem, patientID, other, false)
}

func TestProblemPolicyRequiresAccessToThisPatient(t *testing.T) {
	patientID := uuid.New()
	otherPatientID := uuid.New()
	actor := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeProfessional, HasAccess: true}
	assertProblemAction(t, "other patient", CreateProblem, otherPatientID, actor, false)
	assertProblemAction(t, "no access", ListProblems, patientID, PatientContext{AccountID: actor.AccountID, PatientID: patientID, AccountType: actor.AccountType}, false)
	assertProblemAction(t, "invalid action", ProblemAction("unknown"), patientID, actor, false)
	assertProblemAction(t, "invalid account type", ListProblems, patientID, PatientContext{AccountID: actor.AccountID, PatientID: patientID, HasAccess: true}, false)
	assertProblemAction(t, "missing patient", ListProblems, uuid.Nil, actor, false)
}

func TestProblemPolicyDoesNotTreatUnverifiedLegacyLinksAsIdentity(t *testing.T) {
	patientID := uuid.New()
	actor := PatientContext{AccountID: uuid.New(), PatientID: patientID, AccountType: accountdomain.AccountTypeBasicCare, HasAccess: true}
	assertProblemAction(t, "legacy self or caregiver", ResolveProblem, patientID, actor, false)
	actor.SelfVerified = true
	assertProblemAction(t, "verified self", ResolveProblem, patientID, actor, true)
	actor.SelfVerified = false
	actor.CaregiverAuthorized = true
	assertProblemAction(t, "authorized caregiver", ResolveProblem, patientID, actor, true)
	actor.CaregiverAuthorized = false
	assertProblemAction(t, "revoked context", ResolveProblem, patientID, actor, false)
	actor.CaregiverAuthorized = true
	actor.HasAccess = false
	assertProblemAction(t, "caregiver without access", ResolveProblem, patientID, actor, false)
	actor.HasAccess = true
	assertProblemAction(t, "caregiver in another patient", ResolveProblem, uuid.New(), actor, false)
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
