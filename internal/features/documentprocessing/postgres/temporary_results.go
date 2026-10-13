// internal/features/documentprocessing/postgres/temporary_results.go
package postgres

import (
	"context"
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
	_, err = tx.Exec(ctx, `INSERT INTO public.temporary_extractions
 (id,account_id,kind,result,created_at,expires_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		item.ID, item.AccountID, item.Kind, string(item.Result), item.CreatedAt, item.ExpiresAt)
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
		_, err = tx.Exec(ctx, `INSERT INTO public.temporary_extraction_pages
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
	rows, err := r.client.Pool().Query(ctx, `SELECT e.id,e.kind,e.created_at,e.expires_at
 FROM public.temporary_extractions e WHERE e.account_id=$1 AND e.expires_at>$2
 AND NOT EXISTS (SELECT 1 FROM public.temporary_extraction_pages p
 JOIN public.captures c ON c.id=p.capture_id
 WHERE p.extraction_id=e.id AND (c.status<>'available' OR c.expires_at<=$2))
 ORDER BY created_at DESC LIMIT 100`, accountID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]documentprocessing.TemporaryResult, 0)
	for rows.Next() {
		var item documentprocessing.TemporaryResult
		if err := rows.Scan(&item.ID, &item.Kind, &item.CreatedAt, &item.ExpiresAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TemporaryResultRepository) Get(ctx context.Context, accountID, id uuid.UUID, now time.Time) (*documentprocessing.TemporaryResult, error) {
	var item documentprocessing.TemporaryResult
	err := r.client.Pool().QueryRow(ctx, `SELECT e.id,e.account_id,e.kind,e.result,e.created_at,e.expires_at
 FROM public.temporary_extractions e WHERE e.id=$1 AND e.account_id=$2 AND e.expires_at>$3
 AND NOT EXISTS (SELECT 1 FROM public.temporary_extraction_pages p
 JOIN public.captures c ON c.id=p.capture_id
 WHERE p.extraction_id=e.id AND (c.status<>'available' OR c.expires_at<=$3))`, id, accountID, now).
		Scan(&item.ID, &item.AccountID, &item.Kind, &item.Result, &item.CreatedAt, &item.ExpiresAt)
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
	rows, err := r.client.Pool().Query(ctx, `SELECT id FROM public.temporary_extractions
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
	_, err := r.client.Pool().Exec(ctx, `DELETE FROM public.temporary_extractions WHERE id=$1`, id)
	return err
}

func (r *TemporaryResultRepository) pages(ctx context.Context, id uuid.UUID) ([]documentprocessing.TemporaryPage, error) {
	rows, err := r.client.Pool().Query(ctx, `SELECT position,capture_id,storage_uri,original_filename,mime_type,size_bytes,expires_at
 FROM public.temporary_extraction_pages WHERE extraction_id=$1 ORDER BY position`, id)
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
