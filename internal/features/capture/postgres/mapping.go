// internal/features/capture/postgres/mapping.go
package capturepostgres

import (
	"time"

	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	capturesqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/capture"
	"github.com/jackc/pgx/v5/pgtype"
)

func sessionFromRow(row capturesqlc.CaptureSession) (*capturedomain.Session, error) {
	session := &capturedomain.Session{
		ID: row.ID, AccountID: row.AccountID, PairingCodeHash: cloneBytes(row.PairingCodeHash),
		PairingExpiresAt: row.PairingExpiresAt.Time.UTC(), UploadTokenHash: cloneBytes(row.UploadTokenHash),
		UploadTokenExpiresAt: timePointer(row.UploadTokenExpiresAt), ClaimedAt: timePointer(row.ClaimedAt),
		DesktopLastSeenAt: row.DesktopLastSeenAt.Time.UTC(), MobileLastSeenAt: timePointer(row.MobileLastSeenAt),
		RevokedAt: timePointer(row.RevokedAt), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	return session, session.Validate()
}

func captureFromRow(row capturesqlc.Capture) (*capturedomain.Capture, error) {
	item := &capturedomain.Capture{
		ID: row.ID, AccountID: row.AccountID, CaptureSessionID: row.CaptureSessionID,
		StorageURI: textPointer(row.StorageUri), OriginalFilename: row.OriginalFilename,
		MIMEType: row.MimeType, SizeBytes: row.SizeBytes, Status: capturedomain.Status(row.Status),
		ExpiresAt: row.ExpiresAt.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	return item, item.Validate()
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func nullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func nullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func cloneBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}
