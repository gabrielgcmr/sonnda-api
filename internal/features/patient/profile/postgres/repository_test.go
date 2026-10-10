// internal/features/patient/profile/postgres/repository_test.go
package patientpostgres

import (
	"context"
	"errors"
	"testing"
	"time"

	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	patientsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/patient"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestRepositoryUpdate(t *testing.T) {
	stored := patientRow()
	stored.FullName = "Updated Patient"
	stored.UpdatedAt = pgtype.Timestamptz{Time: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC), Valid: true}
	queries := &stubPatientQueries{updateRow: stored}
	repository := &Repository{queries: queries}
	patient := patientFromRow(patientRow())
	patient.FullName = stored.FullName
	patient.Phone = nil
	patient.CNS = nil

	if err := repository.Update(context.Background(), patient); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if queries.updateParams.FullName != stored.FullName {
		t.Fatalf("Update() full name = %q, want %q", queries.updateParams.FullName, stored.FullName)
	}
	if queries.updateParams.Phone.Valid || queries.updateParams.Cns.Valid {
		t.Fatal("Update() should pass NULL for cleared optional fields")
	}
	if patient.FullName != stored.FullName || !patient.UpdatedAt.Equal(stored.UpdatedAt.Time) {
		t.Fatalf("Update() patient was not refreshed from stored row: %+v", patient)
	}
}

func TestRepositoryUpdateMapsErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "not found", err: pgx.ErrNoRows, want: patientprofile.ErrPatientNotFound},
		{name: "duplicate", err: &pgconn.PgError{Code: "23505"}, want: patientprofile.ErrPatientAlreadyExists},
		{name: "database failure", err: errors.New("database unavailable"), want: persistence.ErrPersistenceFailure},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &Repository{queries: &stubPatientQueries{updateErr: test.err}}
			err := repository.Update(context.Background(), patientFromRow(patientRow()))
			if !errors.Is(err, test.want) {
				t.Fatalf("Update() error = %v, want error matching %v", err, test.want)
			}
		})
	}
}

func TestRepositoryDeletes(t *testing.T) {
	id := uuid.New()
	queries := &stubPatientQueries{softDeleteRows: 1, hardDeleteRows: 1}
	repository := &Repository{queries: queries}

	if err := repository.SoftDelete(context.Background(), id); err != nil {
		t.Fatalf("SoftDelete() error = %v", err)
	}
	if err := repository.HardDelete(context.Background(), id); err != nil {
		t.Fatalf("HardDelete() error = %v", err)
	}

	queries.softDeleteRows = 0
	queries.hardDeleteRows = 0
	if err := repository.SoftDelete(context.Background(), id); !errors.Is(err, patientprofile.ErrPatientNotFound) {
		t.Fatalf("SoftDelete() error = %v, want patient not found", err)
	}
	if err := repository.HardDelete(context.Background(), id); !errors.Is(err, patientprofile.ErrPatientNotFound) {
		t.Fatalf("HardDelete() error = %v, want patient not found", err)
	}

	queries.deleteErr = errors.New("database unavailable")
	if err := repository.SoftDelete(context.Background(), id); !errors.Is(err, persistence.ErrPersistenceFailure) {
		t.Fatalf("SoftDelete() error = %v, want persistence failure", err)
	}
	if err := repository.HardDelete(context.Background(), id); !errors.Is(err, persistence.ErrPersistenceFailure) {
		t.Fatalf("HardDelete() error = %v, want persistence failure", err)
	}
}

func TestRepositoryPatientLists(t *testing.T) {
	row := patientRow()
	queries := &stubPatientQueries{
		findRows:   []patientsqlc.Patient{row},
		listRows:   []patientsqlc.Patient{row},
		searchRows: []patientsqlc.Patient{row},
	}
	repository := &Repository{queries: queries}

	found, err := repository.FindByName(context.Background(), row.FullName)
	if err != nil || len(found) != 1 || found[0].ID != row.ID {
		t.Fatalf("FindByName() = (%+v, %v), want mapped patient", found, err)
	}
	if queries.findName != row.FullName {
		t.Fatalf("FindByName() name = %q, want %q", queries.findName, row.FullName)
	}

	listed, err := repository.List(context.Background(), 25, 5)
	if err != nil || len(listed) != 1 || listed[0].ID != row.ID {
		t.Fatalf("List() = (%+v, %v), want mapped patient", listed, err)
	}
	if queries.listParams.Limit != 25 || queries.listParams.Offset != 5 {
		t.Fatalf("List() params = %+v", queries.listParams)
	}

	searched, err := repository.SearchByName(context.Background(), "patient", 10, 2)
	if err != nil || len(searched) != 1 || searched[0].ID != row.ID {
		t.Fatalf("SearchByName() = (%+v, %v), want mapped patient", searched, err)
	}
	if queries.searchParams.Query.String != "patient" || queries.searchParams.Limit != 10 || queries.searchParams.Offset != 2 {
		t.Fatalf("SearchByName() params = %+v", queries.searchParams)
	}
}

func TestRepositoryListMapsPersistenceError(t *testing.T) {
	repository := &Repository{queries: &stubPatientQueries{listErr: errors.New("database unavailable")}}

	_, err := repository.List(context.Background(), 10, 0)
	if !errors.Is(err, persistence.ErrPersistenceFailure) {
		t.Fatalf("List() error = %v, want persistence failure", err)
	}
}

func patientRow() patientsqlc.Patient {
	createdAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	return patientsqlc.Patient{
		ID:        uuid.New(),
		Cpf:       "12345678901",
		Cns:       pgtype.Text{String: "123456789012345", Valid: true},
		FullName:  "Test Patient",
		BirthDate: pgtype.Date{Time: time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC), Valid: true},
		Gender:    "female",
		Race:      "white",
		Phone:     pgtype.Text{String: "+5511999999999", Valid: true},
		AvatarUrl: pgtype.Text{String: "https://example.com/avatar.png", Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
}

type stubPatientQueries struct {
	findName       string
	findRows       []patientsqlc.Patient
	listParams     patientsqlc.ListPatientsParams
	listRows       []patientsqlc.Patient
	listErr        error
	searchParams   patientsqlc.SearchPatientsByNameParams
	searchRows     []patientsqlc.Patient
	softDeleteRows int64
	hardDeleteRows int64
	deleteErr      error
	updateParams   patientsqlc.UpdatePatientParams
	updateRow      patientsqlc.Patient
	updateErr      error
}

func (q *stubPatientQueries) CreatePatient(context.Context, patientsqlc.CreatePatientParams) (patientsqlc.Patient, error) {
	return patientsqlc.Patient{}, nil
}

func (q *stubPatientQueries) FindPatientsByName(_ context.Context, name string) ([]patientsqlc.Patient, error) {
	q.findName = name
	return q.findRows, nil
}

func (q *stubPatientQueries) GetPatientByCNS(context.Context, pgtype.Text) (patientsqlc.Patient, error) {
	return patientsqlc.Patient{}, nil
}

func (q *stubPatientQueries) GetPatientByCPF(context.Context, string) (patientsqlc.Patient, error) {
	return patientsqlc.Patient{}, nil
}

func (q *stubPatientQueries) GetPatientByID(context.Context, uuid.UUID) (patientsqlc.Patient, error) {
	return patientsqlc.Patient{}, nil
}

func (q *stubPatientQueries) GetPatientByOwnerUserID(context.Context, pgtype.UUID) (patientsqlc.Patient, error) {
	return patientsqlc.Patient{}, nil
}

func (q *stubPatientQueries) HardDeletePatient(context.Context, uuid.UUID) (int64, error) {
	return q.hardDeleteRows, q.deleteErr
}

func (q *stubPatientQueries) ListPatients(_ context.Context, params patientsqlc.ListPatientsParams) ([]patientsqlc.Patient, error) {
	q.listParams = params
	return q.listRows, q.listErr
}

func (q *stubPatientQueries) RestorePatient(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func (q *stubPatientQueries) SearchPatientsByName(_ context.Context, params patientsqlc.SearchPatientsByNameParams) ([]patientsqlc.Patient, error) {
	q.searchParams = params
	return q.searchRows, nil
}

func (q *stubPatientQueries) SoftDeletePatient(context.Context, uuid.UUID) (int64, error) {
	return q.softDeleteRows, q.deleteErr
}

func (q *stubPatientQueries) UpdatePatient(_ context.Context, params patientsqlc.UpdatePatientParams) (patientsqlc.Patient, error) {
	q.updateParams = params
	return q.updateRow, q.updateErr
}
