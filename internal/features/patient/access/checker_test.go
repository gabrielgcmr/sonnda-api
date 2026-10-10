// internal/features/patient/access/checker_test.go
package patientaccess

import (
	"context"
	"errors"
	"testing"

	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type patientLookup struct {
	result *profiledomain.Patient
	err    error
	id     uuid.UUID
}

func (r *patientLookup) FindByID(_ context.Context, id uuid.UUID) (*profiledomain.Patient, error) {
	r.id = id
	return r.result, r.err
}

type accessLookup struct {
	Repository
	allowed   bool
	err       error
	patientID uuid.UUID
	accountID uuid.UUID
}

func (r *accessLookup) HasActiveAccess(_ context.Context, patientID, accountID uuid.UUID) (bool, error) {
	r.patientID, r.accountID = patientID, accountID
	return r.allowed, r.err
}

func TestRequireAccess(t *testing.T) {
	accountID, otherID, patientID := uuid.New(), uuid.New(), uuid.New()
	dbErr := errors.New("database unavailable")
	for _, tc := range []struct {
		name             string
		owner            *uuid.UUID
		allowed          bool
		missing          bool
		patientErr       error
		accessErr        error
		wantKind         apperr.ErrorKind
		wantAccessLookup bool
	}{
		{name: "owner", owner: &accountID},
		{name: "active grant", owner: &otherID, allowed: true, wantAccessLookup: true},
		{name: "no active grant", owner: &otherID, wantKind: apperr.ACCESS_DENIED, wantAccessLookup: true},
		{name: "missing patient", missing: true, allowed: true, wantKind: apperr.ACCESS_DENIED},
		{name: "patient lookup failure", patientErr: dbErr, wantKind: apperr.INFRA_DATABASE_ERROR},
		{name: "access lookup failure", accessErr: dbErr, wantKind: apperr.INFRA_DATABASE_ERROR, wantAccessLookup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patients := &patientLookup{result: &profiledomain.Patient{ID: patientID, OwnerUserID: tc.owner}, err: tc.patientErr}
			if tc.missing {
				patients.result = nil
			}
			access := &accessLookup{allowed: tc.allowed, err: tc.accessErr}
			err := NewChecker(patients, access).RequireAccess(context.Background(), accountID, patientID)
			if tc.wantKind == "" {
				if err != nil {
					t.Fatalf("unexpected denial: %v", err)
				}
			} else {
				var appErr *apperr.AppError
				if !errors.As(err, &appErr) || appErr.Kind != tc.wantKind {
					t.Fatalf("expected %s, got %v", tc.wantKind, err)
				}
				if (tc.patientErr != nil || tc.accessErr != nil) && !errors.Is(err, dbErr) {
					t.Fatalf("database cause was lost: %v", err)
				}
			}
			if patients.id != patientID {
				t.Fatal("wrong patient queried")
			}
			if tc.wantAccessLookup {
				if access.patientID != patientID || access.accountID != accountID {
					t.Fatal("grant checked for the wrong patient or account")
				}
			} else if access.accountID != uuid.Nil {
				t.Fatal("unexpected grant lookup")
			}
		})
	}
}

func TestRequireAccessRejectsMissingIDsAndConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		accountID uuid.UUID
		patientID uuid.UUID
		kind      apperr.ErrorKind
	}{
		{name: "missing account ID", patientID: uuid.New(), kind: apperr.AUTH_REQUIRED},
		{name: "missing patient ID", accountID: uuid.New(), kind: apperr.INTERNAL_ERROR},
		{name: "missing repositories", accountID: uuid.New(), patientID: uuid.New(), kind: apperr.INTERNAL_ERROR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := NewChecker(nil, nil).RequireAccess(context.Background(), tc.accountID, tc.patientID)
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Kind != tc.kind {
				t.Fatalf("expected %s, got %v", tc.kind, err)
			}
		})
	}
}

func TestResolveAccessiblePatientReturnsLoadedEntity(t *testing.T) {
	accountID, patientID := uuid.New(), uuid.New()
	patient := &profiledomain.Patient{ID: patientID, OwnerUserID: &accountID}
	patients := &patientLookup{result: patient}

	resolved, err := NewChecker(patients, &accessLookup{}).ResolveAccessiblePatient(
		context.Background(),
		accountID,
		patientID,
	)
	if err != nil {
		t.Fatalf("unexpected resolution error: %v", err)
	}
	if resolved != patient {
		t.Fatal("resolver did not return the entity loaded during authorization")
	}
}
