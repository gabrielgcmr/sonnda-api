// internal/features/capture/postgres/capture.go
package capturepostgres

import (
	"context"
	"errors"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	capturesqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/capture"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) CreateCapture(ctx context.Context, item capturedomain.Capture) error {
	if err := item.Validate(); err != nil {
		return err
	}
	if r == nil || r.queries == nil {
		return persistenceError("create capture", errors.New("capture repository is not configured"))
	}
	return writeError("create capture", r.queries.CreateCapture(ctx, capturesqlc.CreateCaptureParams{
		ID: item.ID, AccountID: item.AccountID, CaptureSessionID: item.CaptureSessionID,
		StorageUri: nullableText(item.StorageURI), OriginalFilename: item.OriginalFilename,
		MimeType: item.MIMEType, SizeBytes: item.SizeBytes, Status: string(item.Status),
		ExpiresAt: timestamp(item.ExpiresAt), CreatedAt: timestamp(item.CreatedAt), UpdatedAt: timestamp(item.UpdatedAt),
	}))
}

func (r *Repository) SetCaptureAvailable(ctx context.Context, captureID uuid.UUID, storageURI string, updatedAt time.Time) (*capturedomain.Capture, error) {
	row, err := r.queries.SetCaptureAvailable(ctx, capturesqlc.SetCaptureAvailableParams{
		StorageUri: pgtype.Text{String: storageURI, Valid: true}, UpdatedAt: timestamp(updatedAt), ID: captureID,
	})
	if err != nil {
		return nil, resultError("set capture available", err, capture.ErrStateConflict)
	}
	return mapCapture(row)
}

func (r *Repository) MarkCaptureDeleting(ctx context.Context, captureID uuid.UUID, updatedAt time.Time) (*capturedomain.Capture, error) {
	row, err := r.queries.MarkCaptureDeleting(ctx, capturesqlc.MarkCaptureDeletingParams{
		UpdatedAt: timestamp(updatedAt), ID: captureID,
	})
	if err != nil {
		return nil, resultError("mark capture deleting", err, capture.ErrStateConflict)
	}
	return mapCapture(row)
}

func (r *Repository) MarkOwnedCaptureDeleting(ctx context.Context, accountID, captureID uuid.UUID, updatedAt time.Time) (*capturedomain.Capture, error) {
	row, err := r.queries.MarkOwnedCaptureDeleting(ctx, capturesqlc.MarkOwnedCaptureDeletingParams{
		UpdatedAt: timestamp(updatedAt), ID: captureID, AccountID: accountID,
	})
	if err != nil {
		return nil, resultError("mark owned capture deleting", err, capture.ErrCaptureNotFound)
	}
	return mapCapture(row)
}

func (r *Repository) FindCapture(ctx context.Context, accountID, captureID uuid.UUID) (*capturedomain.Capture, error) {
	row, err := r.queries.GetCaptureByAccount(ctx, capturesqlc.GetCaptureByAccountParams{ID: captureID, AccountID: accountID})
	if err != nil {
		return nil, resultError("find capture", err, capture.ErrCaptureNotFound)
	}
	return mapCapture(row)
}

func (r *Repository) FindAvailableCapture(ctx context.Context, accountID, captureID uuid.UUID, now time.Time) (*capturedomain.Capture, error) {
	row, err := r.queries.GetAvailableCaptureByAccount(ctx, capturesqlc.GetAvailableCaptureByAccountParams{
		ID: captureID, AccountID: accountID, Now: timestamp(now),
	})
	if err != nil {
		return nil, resultError("find available capture", err, capture.ErrCaptureNotFound)
	}
	return mapCapture(row)
}

func (r *Repository) ListCaptures(ctx context.Context, accountID uuid.UUID, now time.Time, page capture.Pagination) ([]capturedomain.Capture, error) {
	rows, err := r.queries.ListAvailableCaptures(ctx, capturesqlc.ListAvailableCapturesParams{
		AccountID: accountID, Now: timestamp(now), PageLimit: int32(page.Limit), PageOffset: int32(page.Offset),
	})
	if err != nil {
		return nil, persistenceError("list captures", err)
	}
	return mapCaptures(rows)
}

func (r *Repository) ListCleanupCandidates(
	ctx context.Context,
	now, uploadingCutoff time.Time,
	cursor *capture.CleanupCursor,
	limit int,
) ([]capturedomain.Capture, error) {
	params := capturesqlc.ListCaptureCleanupCandidatesParams{
		Now:             timestamp(now),
		UploadingCutoff: timestamp(uploadingCutoff),
		PageLimit:       int32(limit),
	}
	if cursor != nil {
		params.HasCursor = true
		params.CursorExpiresAt = timestamp(cursor.ExpiresAt)
		params.CursorCreatedAt = timestamp(cursor.CreatedAt)
		params.CursorID = cursor.ID
	}
	rows, err := r.queries.ListCaptureCleanupCandidates(ctx, params)
	if err != nil {
		return nil, persistenceError("list capture cleanup candidates", err)
	}
	return mapCaptures(rows)
}

func (r *Repository) DeleteCapture(ctx context.Context, captureID uuid.UUID) error {
	rows, err := r.queries.DeleteCapture(ctx, captureID)
	return exactlyOne("delete capture", rows, err, capture.ErrCaptureNotFound)
}

func (r *Repository) DeleteOwnedCapture(ctx context.Context, accountID, captureID uuid.UUID) error {
	rows, err := r.queries.DeleteOwnedCapture(ctx, capturesqlc.DeleteOwnedCaptureParams{
		ID: captureID, AccountID: accountID,
	})
	return exactlyOne("delete owned capture", rows, err, capture.ErrCaptureNotFound)
}

func (r *Repository) DeleteExpiredSessions(ctx context.Context, now time.Time, limit int) (int64, error) {
	rows, err := r.queries.DeleteExpiredCaptureSessions(ctx, capturesqlc.DeleteExpiredCaptureSessionsParams{
		Now: timestamp(now), PageLimit: int32(limit),
	})
	return rows, writeError("delete expired capture sessions", err)
}

func mapCapture(row capturesqlc.Capture) (*capturedomain.Capture, error) {
	item, err := captureFromRow(row)
	if err != nil {
		return nil, persistenceError("decode capture", err)
	}
	return item, nil
}

func mapCaptures(rows []capturesqlc.Capture) ([]capturedomain.Capture, error) {
	items := make([]capturedomain.Capture, len(rows))
	for i, row := range rows {
		item, err := captureFromRow(row)
		if err != nil {
			return nil, persistenceError("decode capture", err)
		}
		items[i] = *item
	}
	return items, nil
}
