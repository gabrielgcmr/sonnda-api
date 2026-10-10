// internal/features/patient/profile/authorization_test.go
package patientprofile

import (
	"context"
	"errors"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type recordingPatientResolver struct {
	patient   *profiledomain.Patient
	err       error
	calls     int
	accountID uuid.UUID
	patientID uuid.UUID
}

func (r *recordingPatientResolver) ResolveAccessiblePatient(
	_ context.Context,
	accountID, patientID uuid.UUID,
) (*profiledomain.Patient, error) {
	r.calls++
	r.accountID, r.patientID = accountID, patientID
	return r.patient, r.err
}

func TestProfileAuthorizerReturnsPatientLoadedByAccessResolver(t *testing.T) {
	patient := &profiledomain.Patient{ID: uuid.New()}
	actor := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional}
	resolver := &recordingPatientResolver{patient: patient}

	result, err := NewAuthorizer(resolver).Authorize(context.Background(), actor, patient.ID, HardDeleteProfile)
	if err != nil {
		t.Fatalf("unexpected authorization error: %v", err)
	}
	if result != patient {
		t.Fatal("authorizer did not return the patient loaded by access resolution")
	}
	if resolver.calls != 1 || resolver.accountID != actor.ID || resolver.patientID != patient.ID {
		t.Fatalf("unexpected access resolution: calls=%d account=%s patient=%s", resolver.calls, resolver.accountID, resolver.patientID)
	}
}

func TestProfileAuthorizerRejectsBasicCareDeletionBeforeAccessLookup(t *testing.T) {
	actor := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}
	resolver := &recordingPatientResolver{err: errors.New("resolver should not be called")}
	authorizer := NewAuthorizer(resolver)

	for _, action := range []Action{SoftDeleteProfile, HardDeleteProfile} {
		t.Run(string(action), func(t *testing.T) {
			assertAppErrorKind(t, mustAuthorizeError(authorizer, actor, uuid.New(), action), apperr.ACCESS_DENIED)
		})
	}
	if resolver.calls != 0 {
		t.Fatalf("access resolver called %d times for an account type forbidden by policy", resolver.calls)
	}
}

func TestProfileAuthorizerAllowsBasicCareReadAndUpdateWithAccess(t *testing.T) {
	patient := &profiledomain.Patient{ID: uuid.New()}
	actor := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}
	resolver := &recordingPatientResolver{patient: patient}
	authorizer := NewAuthorizer(resolver)

	for _, action := range []Action{ReadProfile, UpdateProfile} {
		t.Run(string(action), func(t *testing.T) {
			result, err := authorizer.Authorize(context.Background(), actor, patient.ID, action)
			if err != nil || result != patient {
				t.Fatalf("expected authorization, patient=%p result=%p err=%v", patient, result, err)
			}
		})
	}
}

func TestProfileAuthorizerRejectsInvalidAccountsBeforeAccessLookup(t *testing.T) {
	deletedAt := time.Now()
	tests := []struct {
		name    string
		account *accountdomain.Account
		kind    apperr.ErrorKind
	}{
		{name: "missing account", account: nil, kind: apperr.AUTH_REQUIRED},
		{name: "deleted account", account: &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional, DeletedAt: &deletedAt}, kind: apperr.ACCESS_DENIED},
		{name: "invalid account type", account: &accountdomain.Account{ID: uuid.New(), AccountType: "invalid"}, kind: apperr.ACCESS_DENIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &recordingPatientResolver{}
			err := mustAuthorizeError(NewAuthorizer(resolver), tt.account, uuid.New(), ReadProfile)
			assertAppErrorKind(t, err, tt.kind)
			if resolver.calls != 0 {
				t.Fatalf("access resolver called %d times", resolver.calls)
			}
		})
	}
}

func TestProfileAuthorizerPropagatesAccessResolutionFailure(t *testing.T) {
	denied := apperr.Forbidden("acesso negado")
	resolver := &recordingPatientResolver{err: denied}
	actor := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional}

	err := mustAuthorizeError(NewAuthorizer(resolver), actor, uuid.New(), ReadProfile)
	if !errors.Is(err, denied) {
		t.Fatalf("expected access error to be preserved, got %v", err)
	}
}

func TestProfileAuthorizerRequiresConfiguredAccessResolver(t *testing.T) {
	actor := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional}
	err := mustAuthorizeError(NewAuthorizer(nil), actor, uuid.New(), ReadProfile)
	assertAppErrorKind(t, err, apperr.INTERNAL_ERROR)
}

func mustAuthorizeError(authorizer Authorizer, actor *accountdomain.Account, patientID uuid.UUID, action Action) error {
	_, err := authorizer.Authorize(context.Background(), actor, patientID, action)
	return err
}
