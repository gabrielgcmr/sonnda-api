// internal/features/patient/exam/laboratory/authorization_test.go
package laboratory

import (
	"context"
	"errors"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	labdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type laboratoryAccessRecorder struct {
	err       error
	calls     int
	accountID uuid.UUID
	patientID uuid.UUID
}

func (a *laboratoryAccessRecorder) RequireAccess(_ context.Context, accountID, patientID uuid.UUID) error {
	a.calls++
	a.accountID, a.patientID = accountID, patientID
	return a.err
}

type laboratoryReportStore struct {
	Repository
	report *labdomain.LabReport
	calls  int
}

func (s *laboratoryReportStore) FindByID(context.Context, uuid.UUID) (*labdomain.LabReport, error) {
	s.calls++
	return s.report, nil
}

func TestLaboratoryAuthorizerAllowsBothAccountTypesWithPatientAccess(t *testing.T) {
	patientID := uuid.New()
	for _, accountType := range []accountdomain.AccountType{accountdomain.AccountTypeBasicCare, accountdomain.AccountTypeProfessional} {
		t.Run(string(accountType), func(t *testing.T) {
			account := &accountdomain.Account{ID: uuid.New(), AccountType: accountType}
			access := &laboratoryAccessRecorder{}
			err := NewAuthorizer(nil, access).AuthorizePatient(t.Context(), account, patientID, ListPatientReports)
			if err != nil {
				t.Fatalf("unexpected authorization error: %v", err)
			}
			if access.calls != 1 || access.accountID != account.ID || access.patientID != patientID {
				t.Fatalf("unexpected access check: %+v", access)
			}
		})
	}
}

func TestLaboratoryAuthorizerLoadsReportOnceAndAuthorizesItsPatient(t *testing.T) {
	patientID := uuid.New()
	report := &labdomain.LabReport{ID: uuid.New(), PatientID: patientID}
	store := &laboratoryReportStore{report: report}
	access := &laboratoryAccessRecorder{}
	account := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}

	result, err := NewAuthorizer(store, access).AuthorizeReport(t.Context(), account, report.ID, ReadReport)
	if err != nil || result != report {
		t.Fatalf("report authorization failed: result=%p report=%p err=%v", result, report, err)
	}
	if store.calls != 1 || access.calls != 1 || access.patientID != patientID {
		t.Fatalf("unexpected calls: reports=%d access=%d patient=%s", store.calls, access.calls, access.patientID)
	}
}

func TestLaboratoryAuthorizerRejectsInvalidAccountBeforeStores(t *testing.T) {
	deletedAt := time.Now()
	tests := []struct {
		name    string
		account *accountdomain.Account
		kind    apperr.ErrorKind
	}{
		{name: "missing", account: nil, kind: apperr.AUTH_REQUIRED},
		{name: "deleted", account: &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional, DeletedAt: &deletedAt}, kind: apperr.ACCESS_DENIED},
		{name: "invalid type", account: &accountdomain.Account{ID: uuid.New(), AccountType: "invalid"}, kind: apperr.ACCESS_DENIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &laboratoryReportStore{}
			access := &laboratoryAccessRecorder{}
			_, err := NewAuthorizer(store, access).AuthorizeReport(t.Context(), tt.account, uuid.New(), ReadReport)
			assertLaboratoryErrorKind(t, err, tt.kind)
			if store.calls != 0 || access.calls != 0 {
				t.Fatalf("stores called for invalid account: reports=%d access=%d", store.calls, access.calls)
			}
		})
	}
}

func TestLaboratoryAuthorizerPropagatesPatientAccessDenial(t *testing.T) {
	denied := apperr.Forbidden("acesso negado")
	access := &laboratoryAccessRecorder{err: denied}
	account := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional}
	err := NewAuthorizer(nil, access).AuthorizePatient(t.Context(), account, uuid.New(), ReadReport)
	if !errors.Is(err, denied) {
		t.Fatalf("expected access denial, got %v", err)
	}
}

func assertLaboratoryErrorKind(t *testing.T, err error, want apperr.ErrorKind) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Kind != want {
		t.Fatalf("expected %s, got %v", want, err)
	}
}
