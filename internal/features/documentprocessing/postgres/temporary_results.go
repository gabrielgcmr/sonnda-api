// internal/features/documentprocessing/postgres/temporary_results.go
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	pginfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type TemporaryResultRepository struct{ client *pginfra.Client }

func NewTemporaryResultRepository(client *pginfra.Client) *TemporaryResultRepository {
	return &TemporaryResultRepository{client: client}
}

func (r *TemporaryResultRepository) Create(ctx context.Context, item *documentprocessing.TemporaryResult) error {
	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	var result any
	if len(item.Result) > 0 {
		result = string(item.Result)
	}
	_, err = tx.Exec(ctx, `INSERT INTO temporary_extractions
 (id,account_id,kind,result,status,progress,created_at,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		item.ID, item.AccountID, item.Kind, result, item.Status, item.Progress, item.CreatedAt, item.ExpiresAt)
	if err != nil {
		return err
	}
	for _, page := range item.Pages {
		var captureID any
		if page.CaptureID != nil {
			captureID = *page.CaptureID
		}
		var storageURI any
		if page.StorageURI != "" {
			storageURI = page.StorageURI
		}
		_, err = tx.Exec(ctx, `INSERT INTO temporary_extraction_pages
 (extraction_id,position,capture_id,storage_uri,original_filename,mime_type,size_bytes,expires_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, item.ID, page.Position, captureID, storageURI,
			page.OriginalFilename, page.MIMEType, page.SizeBytes, page.ExpiresAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.ConstraintName == "temporary_extraction_pages_capture_unique" {
				return documentprocessing.ErrTemporaryCaptureUsed
			}
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *TemporaryResultRepository) List(ctx context.Context, accountID uuid.UUID, now time.Time) ([]documentprocessing.TemporaryResult, error) {
	rows, err := r.client.Pool().Query(ctx, `SELECT e.id,e.kind,e.status,e.progress,COALESCE(e.error_message,''),
 COALESCE(btrim(e.result->'report'->>'patient_name'),''),COALESCE(btrim(e.result->'report'->>'exam_name'),''),e.created_at,e.expires_at
 FROM temporary_extractions e WHERE e.account_id=$1 AND e.expires_at>$2
 AND NOT EXISTS (SELECT 1 FROM temporary_extraction_pages p
 JOIN captures c ON c.id=p.capture_id
 WHERE p.extraction_id=e.id AND (c.status<>'available' OR c.expires_at<=$2))
 ORDER BY created_at DESC LIMIT 100`, accountID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]documentprocessing.TemporaryResult, 0)
	for rows.Next() {
		var item documentprocessing.TemporaryResult
		if err := rows.Scan(&item.ID, &item.Kind, &item.Status, &item.Progress, &item.ErrorMessage, &item.PatientName, &item.ExamName, &item.CreatedAt, &item.ExpiresAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TemporaryResultRepository) Get(ctx context.Context, accountID, id uuid.UUID, now time.Time) (*documentprocessing.TemporaryResult, error) {
	var item documentprocessing.TemporaryResult
	err := r.client.Pool().QueryRow(ctx, `SELECT e.id,e.account_id,e.kind,e.result,e.status,e.progress,COALESCE(e.error_message,''),e.created_at,e.expires_at
 FROM temporary_extractions e WHERE e.id=$1 AND e.account_id=$2 AND e.expires_at>$3
 AND NOT EXISTS (SELECT 1 FROM temporary_extraction_pages p
 JOIN captures c ON c.id=p.capture_id
 WHERE p.extraction_id=e.id AND (c.status<>'available' OR c.expires_at<=$3))`, id, accountID, now).
		Scan(&item.ID, &item.AccountID, &item.Kind, &item.Result, &item.Status, &item.Progress, &item.ErrorMessage, &item.CreatedAt, &item.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.Pages, err = r.pages(ctx, id)
	return &item, err
}

func (r *TemporaryResultRepository) Expired(ctx context.Context, now time.Time, limit int) ([]documentprocessing.TemporaryResult, error) {
	rows, err := r.client.Pool().Query(ctx, `SELECT id FROM temporary_extractions
 WHERE expires_at<=$1 ORDER BY expires_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	items := make([]documentprocessing.TemporaryResult, 0, len(ids))
	for _, id := range ids {
		pages, err := r.pages(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, documentprocessing.TemporaryResult{ID: id, Pages: pages})
	}
	return items, nil
}

func (r *TemporaryResultRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.client.Pool().Exec(ctx, `DELETE FROM temporary_extractions WHERE id=$1`, id)
	return err
}

func (r *TemporaryResultRepository) Claim(ctx context.Context, owner uuid.UUID, now time.Time, lease time.Duration, maxAttempts int, exhaustedMessage string) (*documentprocessing.TemporaryResult, error) {
	_, err := r.client.Pool().Exec(ctx, `UPDATE temporary_extractions
 SET status='failed',progress=0,error_message=$3,lease_owner=NULL,lease_until=NULL
 WHERE status='processing' AND lease_until<=$1 AND attempts>=$2 AND expires_at>$1`, now, maxAttempts, exhaustedMessage)
	if err != nil {
		return nil, err
	}
	var item documentprocessing.TemporaryResult
	err = r.client.Pool().QueryRow(ctx, `WITH next AS (
 SELECT e.id FROM temporary_extractions e
 WHERE e.expires_at>$1 AND e.attempts<$4 AND (e.status='queued' OR (e.status='processing' AND e.lease_until<=$1))
 ORDER BY e.created_at FOR UPDATE SKIP LOCKED LIMIT 1
 ) UPDATE temporary_extractions e SET status='processing',progress=10,lease_owner=$2,
 lease_until=$3,attempts=e.attempts+1,error_message=NULL FROM next
 WHERE e.id=next.id RETURNING e.id,e.account_id,e.kind,e.status,e.progress,e.created_at,e.expires_at`, now, owner, now.Add(lease), maxAttempts).
		Scan(&item.ID, &item.AccountID, &item.Kind, &item.Status, &item.Progress, &item.CreatedAt, &item.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.Pages, err = r.pages(ctx, item.ID)
	return &item, err
}

func (r *TemporaryResultRepository) Renew(ctx context.Context, id, owner uuid.UUID, now time.Time, lease time.Duration) (bool, error) {
	command, err := r.client.Pool().Exec(ctx, `UPDATE temporary_extractions SET lease_until=$4
 WHERE id=$1 AND lease_owner=$2 AND status='processing' AND lease_until>$3 AND expires_at>$3`, id, owner, now, now.Add(lease))
	return command.RowsAffected() == 1, err
}

func (r *TemporaryResultRepository) Advance(ctx context.Context, id, owner uuid.UUID, now time.Time, progress int) (bool, error) {
	command, err := r.client.Pool().Exec(ctx, `UPDATE temporary_extractions SET progress=$4
 WHERE id=$1 AND lease_owner=$2 AND status='processing' AND lease_until>$3 AND expires_at>$3`, id, owner, now, progress)
	return command.RowsAffected() == 1, err
}

func (r *TemporaryResultRepository) Complete(ctx context.Context, id, owner uuid.UUID, now time.Time, result json.RawMessage) (bool, error) {
	command, err := r.client.Pool().Exec(ctx, `UPDATE temporary_extractions e
 SET result=$4,status='completed',progress=100,lease_owner=NULL,lease_until=NULL,error_message=NULL
 WHERE e.id=$1 AND e.lease_owner=$2 AND e.status='processing' AND e.lease_until>$3 AND e.expires_at>$3
 AND NOT EXISTS (SELECT 1 FROM temporary_extraction_pages p LEFT JOIN captures c ON c.id=p.capture_id
   WHERE p.extraction_id=e.id AND (p.expires_at<=$3 OR (p.capture_id IS NOT NULL AND
   (c.id IS NULL OR c.status<>'available' OR c.expires_at<=$3))))`, id, owner, now, string(result))
	return command.RowsAffected() == 1, err
}

func (r *TemporaryResultRepository) Fail(ctx context.Context, id, owner uuid.UUID, now time.Time, message string) (bool, error) {
	command, err := r.client.Pool().Exec(ctx, `UPDATE temporary_extractions
 SET status='failed',progress=0,error_message=$4,lease_owner=NULL,lease_until=NULL
 WHERE id=$1 AND lease_owner=$2 AND status='processing' AND lease_until>$3 AND expires_at>$3`, id, owner, now, message)
	return command.RowsAffected() == 1, err
}

func (r *TemporaryResultRepository) Release(ctx context.Context, id, owner uuid.UUID) (bool, error) {
	command, err := r.client.Pool().Exec(ctx, `UPDATE temporary_extractions
 SET status='queued',progress=0,lease_owner=NULL,lease_until=NULL,attempts=GREATEST(attempts-1,0)
 WHERE id=$1 AND lease_owner=$2 AND status='processing'`, id, owner)
	return command.RowsAffected() == 1, err
}

func (r *TemporaryResultRepository) Retry(ctx context.Context, accountID, id uuid.UUID, now time.Time) (bool, error) {
	command, err := r.client.Pool().Exec(ctx, `UPDATE temporary_extractions e
 SET status='queued',progress=0,error_message=NULL,lease_owner=NULL,lease_until=NULL,attempts=0
 WHERE e.id=$1 AND e.account_id=$2 AND e.status='failed' AND e.expires_at>$3
 AND NOT EXISTS (SELECT 1 FROM temporary_extraction_pages p LEFT JOIN captures c ON c.id=p.capture_id
   WHERE p.extraction_id=e.id AND (p.expires_at<=$3 OR (p.capture_id IS NOT NULL AND
   (c.id IS NULL OR c.status<>'available' OR c.expires_at<=$3))))`, id, accountID, now)
	return command.RowsAffected() == 1, err
}

func (r *TemporaryResultRepository) pages(ctx context.Context, id uuid.UUID) ([]documentprocessing.TemporaryPage, error) {
	rows, err := r.client.Pool().Query(ctx, `SELECT position,capture_id,storage_uri,original_filename,mime_type,size_bytes,expires_at
 FROM temporary_extraction_pages WHERE extraction_id=$1 ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := make([]documentprocessing.TemporaryPage, 0)
	for rows.Next() {
		var page documentprocessing.TemporaryPage
		var captureID pgtype.UUID
		var storageURI pgtype.Text
		if err := rows.Scan(&page.Position, &captureID, &storageURI, &page.OriginalFilename, &page.MIMEType, &page.SizeBytes, &page.ExpiresAt); err != nil {
			return nil, err
		}
		page.CaptureID = pgUUIDToNullableUUID(captureID)
		if storageURI.Valid {
			page.StorageURI = storageURI.String
		}
		pages = append(pages, page)
	}
	return pages, rows.Err()
}
