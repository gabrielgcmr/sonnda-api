// internal/application/usecase/patientcreation/create_test.go
package patientcreation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/domain/demographics"
	accessdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/access/domain"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"

	"github.com/google/uuid"
)

type creationRepository struct {
	patient *profiledomain.Patient
	access  *accessdomain.PatientAccess
	err     error
}

func (r *creationRepository) CreateWithInitialAccess(
	_ context.Context,
	patient *profiledomain.Patient,
	access *accessdomain.PatientAccess,
) error {
	r.patient = patient
	r.access = access
	return r.err
}

func validInput(relationType string) Input {
	return Input{
		Profile: patientprofile.CreateInput{
			CPF:       "52998224725",
			FullName:  "Joana Silva",
			BirthDate: time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC),
			Gender:    demographics.GenderFemale,
			Race:      demographics.RaceWhite,
			AvatarURL: "https://example.com/avatar.png",
		},
		Access: AccessInput{RelationType: relationType},
	}
}

func TestExecuteUsesRelationshipProvidedByCreator(t *testing.T) {
	repo := &creationRepository{}
	creatorID := uuid.New()

	patient, err := New(repo).Execute(context.Background(), creatorID, validInput("family"))
	if err != nil {
		t.Fatal(err)
	}
	if repo.patient != patient || repo.access == nil {
		t.Fatal("expected patient and initial access to be persisted together")
	}
	if repo.access.GranteeID != creatorID || repo.access.RelationType != accessdomain.RelationshipTypeFamily {
		t.Fatalf("unexpected initial access: %+v", repo.access)
	}
	if repo.access.GrantedBy == nil || *repo.access.GrantedBy != creatorID {
		t.Fatalf("expected creator %s as grantor", creatorID)
	}
	if patient.OwnerUserID != nil {
		t.Fatalf("non-self relationship must not set patient owner: %v", patient.OwnerUserID)
	}
}

func TestExecuteSelfRelationshipSetsCreatorAsOwner(t *testing.T) {
	repo := &creationRepository{}
	creatorID := uuid.New()

	patient, err := New(repo).Execute(context.Background(), creatorID, validInput("self"))
	if err != nil {
		t.Fatal(err)
	}
	if patient.OwnerUserID == nil || *patient.OwnerUserID != creatorID {
		t.Fatalf("expected creator %s as patient owner", creatorID)
	}
}

func TestExecuteRequiresExplicitRelationship(t *testing.T) {
	for _, relationType := range []string{"", "unknown"} {
		t.Run(relationType, func(t *testing.T) {
			repo := &creationRepository{}
			_, err := New(repo).Execute(context.Background(), uuid.New(), validInput(relationType))

			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Kind != apperr.VALIDATION_FAILED {
				t.Fatalf("expected validation error, got %v", err)
			}
			if repo.patient != nil || repo.access != nil {
				t.Fatal("invalid relationship must stop persistence")
			}
		})
	}
}

func TestExecuteMapsAtomicPersistenceFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind apperr.ErrorKind
	}{
		{name: "duplicate patient", err: patientprofile.ErrPatientAlreadyExists, kind: apperr.RESOURCE_ALREADY_EXISTS},
		{name: "database", err: errors.Join(persistence.ErrPersistenceFailure, errors.New("db down")), kind: apperr.INTERNAL_ERROR},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(&creationRepository{err: test.err}).Execute(
				context.Background(),
				uuid.New(),
				validInput("caregiver"),
			)

			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Kind != test.kind {
				t.Fatalf("expected %s, got %v", test.kind, err)
			}
		})
	}
}
