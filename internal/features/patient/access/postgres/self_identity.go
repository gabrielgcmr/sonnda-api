// internal/features/patient/access/postgres/self_identity.go
package accesspostgres

import (
	"context"
	"errors"
	"fmt"

	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type SelfIdentityRepository struct {
	client *postgress.Client
}

func NewSelfIdentityRepository(client *postgress.Client) *SelfIdentityRepository {
	return &SelfIdentityRepository{client: client}
}

func (r *SelfIdentityRepository) HasActiveSelf(ctx context.Context, patientID, accountID uuid.UUID) (bool, error) {
	var active bool
	err := r.client.Pool().QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM public.patient_self_confirmations c
    JOIN public.patients p ON p.id = c.patient_id AND p.deleted_at IS NULL
    JOIN public.users u ON u.id = c.account_id AND u.deleted_at IS NULL
    WHERE c.patient_id = $1 AND c.account_id = $2 AND c.revoked_at IS NULL
)`, patientID, accountID).Scan(&active)
	if err != nil {
		return false, storageError("find active self confirmation", err)
	}
	return active, nil
}

func (r *SelfIdentityRepository) ConfirmSelf(ctx context.Context, patientID, accountID, professionalID uuid.UUID) error {
	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return storageError("begin self confirmation", err)
	}
	defer tx.Rollback(ctx)

	owner, err := lockPatientAndProfessional(ctx, tx, patientID, professionalID)
	if err != nil {
		return err
	}
	if owner.Valid && owner.Bytes != accountID {
		return patientaccess.ErrSelfIdentityConflict
	}
	var targetID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM public.users WHERE id = $1 AND deleted_at IS NULL FOR SHARE`, accountID).Scan(&targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return patientaccess.ErrSelfIdentityTarget
	}
	if err != nil {
		return storageError("lock self account", err)
	}

	var existingAccount uuid.UUID
	err = tx.QueryRow(ctx, `SELECT account_id FROM public.patient_self_confirmations WHERE patient_id = $1 AND revoked_at IS NULL FOR UPDATE`, patientID).Scan(&existingAccount)
	if err == nil {
		if existingAccount != accountID {
			return patientaccess.ErrSelfIdentityConflict
		}
		return storageError("commit repeated self confirmation", tx.Commit(ctx))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return storageError("check patient self confirmation", err)
	}
	var otherPatient uuid.UUID
	err = tx.QueryRow(ctx, `SELECT patient_id FROM public.patient_self_confirmations WHERE account_id = $1 AND revoked_at IS NULL FOR UPDATE`, accountID).Scan(&otherPatient)
	if err == nil {
		return patientaccess.ErrSelfIdentityConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return storageError("check account self confirmation", err)
	}

	createdAccess, err := ensureSelfAccess(ctx, tx, patientID, accountID, professionalID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.patient_self_confirmations
    (id, patient_id, account_id, confirmed_by, access_created)
    VALUES ($1, $2, $3, $4, $5)`, uuid.New(), patientID, accountID, professionalID, createdAccess)
	if err != nil {
		return identityWriteError("insert self confirmation", err)
	}
	return storageError("commit self confirmation", tx.Commit(ctx))
}

func (r *SelfIdentityRepository) RevokeSelf(ctx context.Context, patientID, accountID, professionalID uuid.UUID) error {
	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return storageError("begin self revocation", err)
	}
	defer tx.Rollback(ctx)
	if _, err := lockPatientAndProfessional(ctx, tx, patientID, professionalID); err != nil {
		return err
	}
	var confirmationID uuid.UUID
	var accessCreated bool
	err = tx.QueryRow(ctx, `SELECT id, access_created FROM public.patient_self_confirmations
    WHERE patient_id = $1 AND account_id = $2 AND revoked_at IS NULL FOR UPDATE`, patientID, accountID).Scan(&confirmationID, &accessCreated)
	if errors.Is(err, pgx.ErrNoRows) {
		return patientaccess.ErrSelfIdentityAbsent
	}
	if err != nil {
		return storageError("lock self confirmation", err)
	}
	_, err = tx.Exec(ctx, `UPDATE public.patient_self_confirmations
    SET revoked_at = now(), revoked_by = $2 WHERE id = $1`, confirmationID, professionalID)
	if err != nil {
		return storageError("revoke self confirmation", err)
	}
	if accessCreated {
		_, err = tx.Exec(ctx, `UPDATE public.patient_access SET revoked_at = now()
    WHERE patient_id = $1 AND grantee_id = $2 AND relation_type = 'self' AND revoked_at IS NULL`, patientID, accountID)
		if err != nil {
			return storageError("revoke self access", err)
		}
	}
	return storageError("commit self revocation", tx.Commit(ctx))
}

func lockPatientAndProfessional(ctx context.Context, tx pgx.Tx, patientID, professionalID uuid.UUID) (pgtype.UUID, error) {
	var owner pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT owner_user_id FROM public.patients
    WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, patientID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return owner, patientaccess.ErrSelfIdentityTarget
	}
	if err != nil {
		return owner, storageError("lock patient", err)
	}
	var accountType string
	err = tx.QueryRow(ctx, `SELECT account_type FROM public.users
    WHERE id = $1 AND deleted_at IS NULL FOR SHARE`, professionalID).Scan(&accountType)
	if errors.Is(err, pgx.ErrNoRows) {
		return owner, patientaccess.ErrSelfIdentityDenied
	}
	if err != nil {
		return owner, storageError("lock professional", err)
	}
	if accountType != "professional" {
		return owner, patientaccess.ErrSelfIdentityDenied
	}
	if owner.Valid && owner.Bytes == professionalID {
		return owner, nil
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT true FROM public.patient_access
    WHERE patient_id = $1 AND grantee_id = $2 AND revoked_at IS NULL FOR SHARE`, patientID, professionalID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return owner, patientaccess.ErrSelfIdentityDenied
	}
	if err != nil {
		return owner, storageError("lock professional access", err)
	}
	return owner, nil
}

func ensureSelfAccess(ctx context.Context, tx pgx.Tx, patientID, accountID, professionalID uuid.UUID) (bool, error) {
	var relation string
	var revoked pgtype.Timestamptz
	err := tx.QueryRow(ctx, `SELECT relation_type, revoked_at FROM public.patient_access
    WHERE patient_id = $1 AND grantee_id = $2 FOR UPDATE`, patientID, accountID).Scan(&relation, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `INSERT INTO public.patient_access
    (patient_id, grantee_id, relation_type, granted_by) VALUES ($1, $2, 'self', $3)`, patientID, accountID, professionalID)
		return true, identityWriteError("grant self access", err)
	}
	if err != nil {
		return false, storageError("lock self access", err)
	}
	if !revoked.Valid {
		return false, nil
	}
	if relation != "self" {
		return false, patientaccess.ErrSelfIdentityConflict
	}
	_, err = tx.Exec(ctx, `UPDATE public.patient_access SET revoked_at = NULL, granted_by = $3
    WHERE patient_id = $1 AND grantee_id = $2`, patientID, accountID, professionalID)
	return true, storageError("reactivate self access", err)
}

func identityWriteError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23514") {
		return patientaccess.ErrSelfIdentityConflict
	}
	return storageError(operation, err)
}

func storageError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(persistence.ErrPersistenceFailure, fmt.Errorf("%s: %w", operation, err))
}

var _ patientaccess.SelfIdentityRepository = (*SelfIdentityRepository)(nil)
