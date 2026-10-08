// internal/features/patient/profile/postgres/repository.go
package patientpostgres

import (
	"context"
	"errors"

	"github.com/gabrielgcmr/sonnda/internal/domain/demographics"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	patientsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/patient"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository struct {
	queries patientsqlc.Querier
}

// FindByName implements [patientprofile.Repository].
func (r *Repository) FindByName(ctx context.Context, name string) ([]profiledomain.Patient, error) {
	rows, err := r.queries.FindPatientsByName(ctx, name)
	if err != nil {
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}
	return patientsFromRows(rows), nil
}

// SearchByName implements [patientprofile.Repository].
func (r *Repository) SearchByName(ctx context.Context, name string, limit int, offset int) ([]profiledomain.Patient, error) {
	rows, err := r.queries.SearchPatientsByName(ctx, patientsqlc.SearchPatientsByNameParams{
		Query:  pgtype.Text{String: name, Valid: true},
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}
	return patientsFromRows(rows), nil
}

// HardDelete implements [patientprofile.Repository].
func (r *Repository) HardDelete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.HardDeletePatient(ctx, id)
	if err != nil {
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}
	if rows == 0 {
		return patientprofile.ErrPatientNotFound
	}
	return nil
}

// List implements [patientprofile.Repository].
func (r *Repository) List(ctx context.Context, limit int, offset int) ([]profiledomain.Patient, error) {
	rows, err := r.queries.ListPatients(ctx, patientsqlc.ListPatientsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}
	return patientsFromRows(rows), nil
}

// Update implements [patientprofile.Repository].
func (r *Repository) Update(ctx context.Context, patient *profiledomain.Patient) error {
	row, err := r.queries.UpdatePatient(ctx, patientsqlc.UpdatePatientParams{
		ID:        patient.ID,
		FullName:  patient.FullName,
		Phone:     fromNullableString(patient.Phone),
		AvatarUrl: fromNullableString(&patient.AvatarURL),
		Gender:    string(patient.Gender),
		Race:      string(patient.Race),
		Cns:       fromNullableString(patient.CNS),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return patientprofile.ErrPatientNotFound
		}
		if isUniqueViolation(err) {
			return patientprofile.ErrPatientAlreadyExists
		}
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}

	*patient = *patientFromRow(row)
	return nil
}

var _ patientprofile.Repository = (*Repository)(nil)

func NewRepository(client *postgress.Client) patientprofile.Repository {
	return &Repository{
		queries: patientsqlc.New(client.Pool()),
	}
}

// Create implements [patientprofile.Repository].
func (r *Repository) Create(ctx context.Context, p *profiledomain.Patient) error {
	return r.createWithQueries(ctx, r.queries, p)
}

func (r *Repository) createWithQueries(
	ctx context.Context,
	queries patientsqlc.Querier,
	p *profiledomain.Patient,
) error {
	params := patientsqlc.CreatePatientParams{
		ID:          p.ID,
		OwnerUserID: fromNullableUUID(p.OwnerUserID),
		Cpf:         p.CPF,
		Cns:         fromNullableString(p.CNS),
		FullName:    p.FullName,
		BirthDate:   pgtype.Date{Time: p.BirthDate, Valid: true},
		Gender:      string(p.Gender),
		Race:        string(p.Race),
		Phone:       fromNullableString(p.Phone),
		AvatarUrl:   fromNullableString(&p.AvatarURL),
	}

	row, err := queries.CreatePatient(ctx, params)
	if err != nil {
		if isUniqueViolation(err) {
			return patientprofile.ErrPatientAlreadyExists
		}
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}

	*p = *patientFromRow(row)

	return nil
}

// SoftDelete implements [patientprofile.Repository].
func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.SoftDeletePatient(ctx, id)
	if err != nil {
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}
	if rows == 0 {
		return patientprofile.ErrPatientNotFound
	}
	return nil
}

// FindByCPF implements [patientprofile.Repository].
func (p *Repository) FindByCPF(ctx context.Context, cpf string) (*profiledomain.Patient, error) {
	row, err := p.queries.GetPatientByCPF(ctx, cpf)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}

	return patientFromRow(row), nil
}

// FindByID implements [patientprofile.Repository].
func (p *Repository) FindByID(ctx context.Context, id uuid.UUID) (*profiledomain.Patient, error) {
	row, err := p.queries.GetPatientByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}

	return patientFromRow(row), nil
}

func patientFromRow(row patientsqlc.Patient) *profiledomain.Patient {
	return &profiledomain.Patient{
		ID:          row.ID,
		OwnerUserID: fromPgUUID(row.OwnerUserID),
		CPF:         row.Cpf,
		CNS:         fromPgText(row.Cns),
		FullName:    row.FullName,
		BirthDate:   row.BirthDate.Time,
		Gender:      demographics.Gender(row.Gender),
		Race:        demographics.Race(row.Race),
		AvatarURL:   row.AvatarUrl.String,
		Phone:       fromPgText(row.Phone),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func patientsFromRows(rows []patientsqlc.Patient) []profiledomain.Patient {
	patients := make([]profiledomain.Patient, len(rows))
	for i, row := range rows {
		patients[i] = *patientFromRow(row)
	}
	return patients
}

func fromNullableString(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func fromPgText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func fromNullableUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil || *value == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *value, Valid: true}
}

func fromPgUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	result := uuid.UUID(value.Bytes)
	return &result
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
