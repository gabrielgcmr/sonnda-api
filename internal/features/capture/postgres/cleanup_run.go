// internal/features/capture/postgres/cleanup_run.go
package capturepostgres

import (
	"context"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturesqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/capture"
)

func (r *Repository) CreateCleanupRun(ctx context.Context, run capture.CleanupRun) error {
	err := r.queries.CreateCaptureCleanupRun(ctx, capturesqlc.CreateCaptureCleanupRunParams{
		ID:        run.ID,
		StartedAt: timestamp(run.StartedAt),
	})
	return writeError("create capture cleanup run", err)
}

func (r *Repository) FinishCleanupRun(ctx context.Context, completion capture.CleanupRunCompletion) error {
	status := "failed"
	if completion.Succeeded {
		status = "succeeded"
	}
	rows, err := r.queries.FinishCaptureCleanupRun(ctx, capturesqlc.FinishCaptureCleanupRunParams{
		ID:                completion.ID,
		Status:            status,
		CapturesProcessed: int64(completion.CapturesProcessed),
		CapturesDeleted:   int64(completion.CapturesDeleted),
		StorageDeleted:    int64(completion.StorageDeleted),
		SessionsDeleted:   int64(completion.SessionsDeleted),
		ErrorCount:        int32(completion.ErrorCount),
		FinishedAt:        timestamp(completion.FinishedAt),
	})
	return exactlyOne("finish capture cleanup run", rows, err, capture.ErrStateConflict)
}
