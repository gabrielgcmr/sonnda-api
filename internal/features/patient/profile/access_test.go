// internal/features/patient/profile/access_test.go
package patientprofile

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type deniedPatientAccess struct {
	err       error
	accountID uuid.UUID
	patientID uuid.UUID
}

func (a *deniedPatientAccess) RequireAccess(_ context.Context, accountID, patientID uuid.UUID) error {
	a.accountID, a.patientID = accountID, patientID
	return a.err
}

func TestPatientOperationsStopWhenAccessIsDenied(t *testing.T) {
	actor := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional}
	patientID := uuid.New()
	denied := apperr.Forbidden("acesso negado")
	for _, operation := range []string{"get", "update", "soft delete", "hard delete"} {
		t.Run(operation, func(t *testing.T) {
			accessChecker := &deniedPatientAccess{err: denied}
			// Nil repositories ensure no read or write happens after denial.
			svc := New(nil, accessChecker)
			var err error
			switch operation {
			case "get":
				_, err = svc.Get(context.Background(), actor, patientID)
			case "update":
				_, err = svc.Update(context.Background(), actor, patientID, UpdateInput{})
			case "soft delete":
				err = svc.SoftDelete(context.Background(), actor, patientID)
			case "hard delete":
				err = svc.HardDelete(context.Background(), actor, patientID)
			}
			if !errors.Is(err, denied) {
				t.Fatalf("expected access denial, got %v", err)
			}
			if accessChecker.accountID != actor.ID || accessChecker.patientID != patientID {
				t.Fatal("wrong access check")
			}
		})
	}
}

type deletionRepository struct {
	Repository
	patient         *profiledomain.Patient
	softDeleteCalls int
	hardDeleteCalls int
}

func (r *deletionRepository) FindByID(context.Context, uuid.UUID) (*profiledomain.Patient, error) {
	return r.patient, nil
}

func (r *deletionRepository) SoftDelete(context.Context, uuid.UUID) error {
	r.softDeleteCalls++
	return nil
}

func (r *deletionRepository) HardDelete(context.Context, uuid.UUID) error {
	r.hardDeleteCalls++
	return nil
}

func TestPatientDeletesRequireProfessionalAccount(t *testing.T) {
	patientID := uuid.New()
	for _, operation := range []string{"soft delete", "hard delete"} {
		t.Run(operation, func(t *testing.T) {
			accessChecker := &deniedPatientAccess{err: errors.New("access checker should not be called")}
			svc := New(nil, accessChecker)
			actor := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}

			var err error
			if operation == "soft delete" {
				err = svc.SoftDelete(context.Background(), actor, patientID)
			} else {
				err = svc.HardDelete(context.Background(), actor, patientID)
			}

			assertAppErrorKind(t, err, apperr.ACCESS_DENIED)
			if accessChecker.accountID != uuid.Nil {
				t.Fatal("access checker called for a non-professional account")
			}
		})
	}
}

func TestPatientDeletesRequireAuthentication(t *testing.T) {
	patientID := uuid.New()
	accessChecker := &deniedPatientAccess{err: errors.New("access checker should not be called")}
	svc := New(nil, accessChecker)

	for _, operation := range []string{"soft delete", "hard delete"} {
		t.Run(operation, func(t *testing.T) {
			var err error
			if operation == "soft delete" {
				err = svc.SoftDelete(context.Background(), nil, patientID)
			} else {
				err = svc.HardDelete(context.Background(), nil, patientID)
			}
			assertAppErrorKind(t, err, apperr.AUTH_REQUIRED)
		})
	}

	if accessChecker.accountID != uuid.Nil {
		t.Fatal("access checker called without an authenticated account")
	}
}

func TestProfessionalWithAccessCanDeletePatient(t *testing.T) {
	patientID := uuid.New()
	actor := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional}

	for _, operation := range []string{"soft delete", "hard delete"} {
		t.Run(operation, func(t *testing.T) {
			repository := &deletionRepository{patient: &profiledomain.Patient{ID: patientID}}
			accessChecker := &deniedPatientAccess{}
			svc := New(repository, accessChecker)

			var err error
			if operation == "soft delete" {
				err = svc.SoftDelete(context.Background(), actor, patientID)
			} else {
				err = svc.HardDelete(context.Background(), actor, patientID)
			}
			if err != nil {
				t.Fatalf("unexpected delete error: %v", err)
			}
			if accessChecker.accountID != actor.ID || accessChecker.patientID != patientID {
				t.Fatal("professional access was not checked")
			}
			if operation == "soft delete" && repository.softDeleteCalls != 1 {
				t.Fatal("soft delete was not executed")
			}
			if operation == "hard delete" && repository.hardDeleteCalls != 1 {
				t.Fatal("hard delete was not executed")
			}
		})
	}
}

func assertAppErrorKind(t *testing.T, err error, want apperr.ErrorKind) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Kind != want {
		t.Fatalf("expected %s, got %v", want, err)
	}
}
