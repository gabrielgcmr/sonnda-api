// internal/features/capture/postgres/session.go
package capturepostgres

import (
	"context"
	"errors"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	capturesqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/capture"
	"github.com/google/uuid"
)

func (r *Repository) CreateSession(ctx context.Context, session capturedomain.Session) error {
	if err := session.Validate(); err != nil {
		return err
	}
	if r == nil || r.queries == nil {
		return persistenceError("create capture session", errors.New("capture repository is not configured"))
	}
	return writeError("create capture session", r.queries.CreateCaptureSession(ctx, capturesqlc.CreateCaptureSessionParams{
		ID: session.ID, AccountID: session.AccountID, PairingCodeHash: cloneBytes(session.PairingCodeHash),
		PairingExpiresAt: timestamp(session.PairingExpiresAt), UploadTokenHash: cloneBytes(session.UploadTokenHash),
		UploadTokenExpiresAt: nullableTimestamp(session.UploadTokenExpiresAt), ClaimedAt: nullableTimestamp(session.ClaimedAt),
		DesktopLastSeenAt: timestamp(session.DesktopLastSeenAt), MobileLastSeenAt: nullableTimestamp(session.MobileLastSeenAt),
		RevokedAt: nullableTimestamp(session.RevokedAt), CreatedAt: timestamp(session.CreatedAt), UpdatedAt: timestamp(session.UpdatedAt),
	}))
}

func (r *Repository) FindSession(ctx context.Context, id uuid.UUID) (*capturedomain.Session, error) {
	if r == nil || r.queries == nil {
		return nil, persistenceError("find capture session", errors.New("capture repository is not configured"))
	}
	row, err := r.queries.GetCaptureSessionByID(ctx, id)
	if err != nil {
		return nil, resultError("find capture session", err, capture.ErrSessionNotFound)
	}
	return mapSession(row)
}

func (r *Repository) FindCurrentSession(ctx context.Context, accountID uuid.UUID) (*capturedomain.Session, error) {
	if r == nil || r.queries == nil {
		return nil, persistenceError("find current capture session", errors.New("capture repository is not configured"))
	}
	row, err := r.queries.GetCurrentCaptureSession(ctx, accountID)
	if err != nil {
		return nil, resultError("find current capture session", err, capture.ErrSessionNotFound)
	}
	return mapSession(row)
}

func (r *Repository) RevokeSessionsByAccount(ctx context.Context, accountID uuid.UUID, revokedAt time.Time) (int64, error) {
	rows, err := r.queries.RevokeCaptureSessionsByAccount(ctx, capturesqlc.RevokeCaptureSessionsByAccountParams{
		RevokedAt: timestamp(revokedAt), AccountID: accountID,
	})
	return rows, writeError("revoke capture sessions", err)
}

func (r *Repository) ClaimSession(ctx context.Context, pairingHash []byte, claim capturedomain.SessionClaim, desktopPresenceCutoff time.Time) (*capturedomain.Session, error) {
	if err := claim.Validate(); err != nil {
		return nil, err
	}
	if len(pairingHash) != capturedomain.CredentialHashBytes {
		return nil, capturedomain.ErrInvalidHash
	}
	row, err := r.queries.ClaimCaptureSession(ctx, capturesqlc.ClaimCaptureSessionParams{
		UploadTokenHash: cloneBytes(claim.UploadTokenHash), UploadTokenExpiresAt: timestamp(claim.UploadTokenExpiresAt),
		ClaimedAt: timestamp(claim.ClaimedAt), PairingCodeHash: cloneBytes(pairingHash),
		DesktopPresenceCutoff: timestamp(desktopPresenceCutoff),
	})
	if err != nil {
		return nil, resultError("claim capture session", err, capture.ErrStateConflict)
	}
	return mapSession(row)
}

func (r *Repository) AuthenticateMobile(ctx context.Context, uploadTokenHash []byte, now time.Time) (*capturedomain.Session, error) {
	if len(uploadTokenHash) != capturedomain.CredentialHashBytes {
		return nil, capturedomain.ErrInvalidHash
	}
	row, err := r.queries.AuthenticateCaptureMobile(ctx, capturesqlc.AuthenticateCaptureMobileParams{
		UploadTokenHash: cloneBytes(uploadTokenHash), Now: timestamp(now),
	})
	if err != nil {
		return nil, resultError("authenticate capture mobile", err, capture.ErrSessionNotFound)
	}
	return mapSession(row)
}

func (r *Repository) AuthenticateUpload(ctx context.Context, uploadTokenHash []byte, now, desktopPresenceCutoff time.Time) (*capturedomain.Session, error) {
	if len(uploadTokenHash) != capturedomain.CredentialHashBytes {
		return nil, capturedomain.ErrInvalidHash
	}
	row, err := r.queries.AuthenticateCaptureUpload(ctx, capturesqlc.AuthenticateCaptureUploadParams{
		UploadTokenHash: cloneBytes(uploadTokenHash), Now: timestamp(now),
		DesktopPresenceCutoff: timestamp(desktopPresenceCutoff),
	})
	if err != nil {
		return nil, resultError("authenticate capture upload", err, capture.ErrSessionNotFound)
	}
	return mapSession(row)
}

func (r *Repository) TouchDesktop(ctx context.Context, sessionID, accountID uuid.UUID, seenAt time.Time) error {
	rows, err := r.queries.TouchCaptureSessionDesktop(ctx, capturesqlc.TouchCaptureSessionDesktopParams{
		SeenAt: timestamp(seenAt), ID: sessionID, AccountID: accountID,
	})
	return exactlyOne("touch capture desktop presence", rows, err, capture.ErrSessionNotFound)
}

func (r *Repository) TouchMobile(ctx context.Context, sessionID uuid.UUID, uploadTokenHash []byte, seenAt time.Time) error {
	if len(uploadTokenHash) != capturedomain.CredentialHashBytes {
		return capturedomain.ErrInvalidHash
	}
	rows, err := r.queries.TouchCaptureSessionMobile(ctx, capturesqlc.TouchCaptureSessionMobileParams{
		SeenAt: timestamp(seenAt), ID: sessionID, UploadTokenHash: cloneBytes(uploadTokenHash),
	})
	return exactlyOne("touch capture mobile presence", rows, err, capture.ErrSessionNotFound)
}

func (r *Repository) RevokeSession(ctx context.Context, sessionID, accountID uuid.UUID, revokedAt time.Time) error {
	rows, err := r.queries.RevokeCaptureSession(ctx, capturesqlc.RevokeCaptureSessionParams{
		RevokedAt: timestamp(revokedAt), ID: sessionID, AccountID: accountID,
	})
	return exactlyOne("revoke capture session", rows, err, capture.ErrSessionNotFound)
}

func mapSession(row capturesqlc.CaptureSession) (*capturedomain.Session, error) {
	session, err := sessionFromRow(row)
	if err != nil {
		return nil, persistenceError("decode capture session", err)
	}
	return session, nil
}

func exactlyOne(operation string, rows int64, err, notFound error) error {
	if err != nil {
		return writeError(operation, err)
	}
	if rows != 1 {
		return notFound
	}
	return nil
}
