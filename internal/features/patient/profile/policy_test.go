// internal/features/patient/profile/policy_test.go
package patientprofile

import (
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

func TestProfilePolicyAllowsAccessibleAccountActions(t *testing.T) {
	patientID := uuid.New()
	tests := []struct {
		name        string
		accountType accountdomain.AccountType
		action      Action
	}{
		{name: "basic care reads", accountType: accountdomain.AccountTypeBasicCare, action: ReadProfile},
		{name: "basic care updates", accountType: accountdomain.AccountTypeBasicCare, action: UpdateProfile},
		{name: "professional reads", accountType: accountdomain.AccountTypeProfessional, action: ReadProfile},
		{name: "professional updates", accountType: accountdomain.AccountTypeProfessional, action: UpdateProfile},
		{name: "professional soft deletes", accountType: accountdomain.AccountTypeProfessional, action: SoftDeleteProfile},
		{name: "professional hard deletes", accountType: accountdomain.AccountTypeProfessional, action: HardDeleteProfile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireAction(tt.action, patientID, authz.PatientContext{
				AccountID:   uuid.New(),
				PatientID:   patientID,
				AccountType: tt.accountType,
				HasAccess:   true,
			})
			if err != nil {
				t.Fatalf("expected action to be allowed, got %v", err)
			}
		})
	}
}

func TestProfilePolicyRejectsBasicCareDeletion(t *testing.T) {
	patientID := uuid.New()
	for _, action := range []Action{SoftDeleteProfile, HardDeleteProfile} {
		t.Run(string(action), func(t *testing.T) {
			err := RequireAction(action, patientID, authz.PatientContext{
				AccountID:   uuid.New(),
				PatientID:   patientID,
				AccountType: accountdomain.AccountTypeBasicCare,
				HasAccess:   true,
			})
			assertAppErrorKind(t, err, apperr.ACCESS_DENIED)
		})
	}
}

func TestProfilePolicyRejectsInvalidAuthorizationFacts(t *testing.T) {
	patientID := uuid.New()
	valid := authz.PatientContext{
		AccountID:   uuid.New(),
		PatientID:   patientID,
		AccountType: accountdomain.AccountTypeProfessional,
		HasAccess:   true,
	}
	tests := []struct {
		name string
		kind apperr.ErrorKind
		edit func(*authz.PatientContext) (Action, uuid.UUID)
	}{
		{name: "missing account", kind: apperr.AUTH_REQUIRED, edit: func(actor *authz.PatientContext) (Action, uuid.UUID) {
			actor.AccountID = uuid.Nil
			return ReadProfile, patientID
		}},
		{name: "invalid account type", kind: apperr.ACCESS_DENIED, edit: func(actor *authz.PatientContext) (Action, uuid.UUID) {
			actor.AccountType = "invalid"
			return ReadProfile, patientID
		}},
		{name: "missing access", kind: apperr.ACCESS_DENIED, edit: func(actor *authz.PatientContext) (Action, uuid.UUID) {
			actor.HasAccess = false
			return ReadProfile, patientID
		}},
		{name: "different patient", kind: apperr.ACCESS_DENIED, edit: func(actor *authz.PatientContext) (Action, uuid.UUID) {
			actor.PatientID = uuid.New()
			return ReadProfile, patientID
		}},
		{name: "missing patient", kind: apperr.ACCESS_DENIED, edit: func(actor *authz.PatientContext) (Action, uuid.UUID) {
			return ReadProfile, uuid.Nil
		}},
		{name: "unknown action", kind: apperr.ACCESS_DENIED, edit: func(actor *authz.PatientContext) (Action, uuid.UUID) {
			return Action("unknown"), patientID
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := valid
			action, targetPatientID := tt.edit(&actor)
			assertAppErrorKind(t, RequireAction(action, targetPatientID, actor), tt.kind)
		})
	}
}
