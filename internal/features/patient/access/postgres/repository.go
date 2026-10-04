// internal/features/patient/access/postgres/repository.go
package accesspostgres

import (
	"context"
	"errors"
	"fmt"

	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	accessdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/access/domain"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	patientaccesssqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/patientaccess"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository struct {
	client  *postgress.Client
	queries *patientaccesssqlc.Queries
}

var _ patientaccess.Repository = (*Repository)(nil)

func NewRepository(client *postgress.Client) *Repository {
	return &Repository{
		client:  client,
		queries: patientaccesssqlc.New(client.Pool()),
	}
}

// ListAccessiblePatientsByUser implements [patientaccess.Repository].
func (p *Repository) ListAccessiblePatientsByUser(ctx context.Context, granteeID uuid.UUID, limit, offset int) ([]patientaccess.AccessiblePatient, int64, error) {
	// Buscar lista paginada
	rows, err := p.queries.ListAccessiblePatientsByUser(ctx, patientaccesssqlc.ListAccessiblePatientsByUserParams{
		GranteeID: pgtype.UUID{Bytes: granteeID, Valid: true},
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, errors.Join(
			persistence.ErrPersistenceFailure,
			fmt.Errorf("list accessible patients: %w", err),
		)
	}

	// Buscar count total
	total, err := p.queries.CountAccessiblePatientsByUser(ctx, pgtype.UUID{Bytes: granteeID, Valid: true})
	if err != nil {
		return nil, 0, errors.Join(
			persistence.ErrPersistenceFailure,
			fmt.Errorf("count accessible patients: %w", err),
		)
	}

	// Mapear para DTO
	result := make([]patientaccess.AccessiblePatient, len(rows))
	for i, row := range rows {
		var avatarURL *string
		if row.AvatarUrl.Valid {
			avatarURL = &row.AvatarUrl.String
		}

		result[i] = patientaccess.AccessiblePatient{
			PatientID:    row.PatientID.Bytes,
			FullName:     row.FullName,
			AvatarURL:    avatarURL,
			RelationType: row.RelationType,
		}
	}

	return result, total, nil
}

// HasActiveAccess implements [patientaccess.Repository].
func (p *Repository) HasActiveAccess(ctx context.Context, patientID uuid.UUID, granteeID uuid.UUID) (bool, error) {
	access, err := p.queries.FindPatientAccess(ctx, patientaccesssqlc.FindPatientAccessParams{
		PatientID: pgtype.UUID{Bytes: patientID, Valid: true},
		GranteeID: pgtype.UUID{Bytes: granteeID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, errors.Join(
			persistence.ErrPersistenceFailure,
			fmt.Errorf("find patient access: %w", err),
		)
	}

	// Verifica se não está revogado
	return !access.RevokedAt.Valid, nil
}

func (p *Repository) FindActiveRelationship(ctx context.Context, patientID, granteeID uuid.UUID) (*accessdomain.RelationshipType, error) {
	access, err := p.queries.FindPatientAccess(ctx, patientaccesssqlc.FindPatientAccessParams{
		PatientID: pgtype.UUID{Bytes: patientID, Valid: true},
		GranteeID: pgtype.UUID{Bytes: granteeID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Join(persistence.ErrPersistenceFailure, fmt.Errorf("find patient relationship: %w", err))
	}
	if access.RevokedAt.Valid {
		return nil, nil
	}
	relation := accessdomain.RelationshipType(access.RelationType)
	if !relation.IsValid() {
		return nil, errors.Join(persistence.ErrPersistenceFailure, fmt.Errorf("invalid patient relationship %q", access.RelationType))
	}
	return &relation, nil
}

// Upsert implements [patientaccess.Repository].
func (p *Repository) Upsert(ctx context.Context, access *accessdomain.PatientAccess) error {
	if err := access.Validate(); err != nil {
		return fmt.Errorf("invalid patient access: %w", err)
	}

	var grantedBy pgtype.UUID
	if access.GrantedBy != nil {
		grantedBy = pgtype.UUID{Bytes: *access.GrantedBy, Valid: true}
	}

	err := p.queries.UpsertPatientAccess(ctx, patientaccesssqlc.UpsertPatientAccessParams{
		PatientID:    pgtype.UUID{Bytes: access.PatientID, Valid: true},
		GranteeID:    pgtype.UUID{Bytes: access.GranteeID, Valid: true},
		RelationType: string(access.RelationType),
		GrantedBy:    grantedBy,
	})
	if err != nil {
		return errors.Join(
			persistence.ErrPersistenceFailure,
			fmt.Errorf("upsert patient access: %w", err),
		)
	}

	return nil
}
