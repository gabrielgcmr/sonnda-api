// internal/features/capture/repository.go
package capture

import (
	"context"
	"errors"
	"io"
	"time"

	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	"github.com/google/uuid"
)

var (
	ErrSessionNotFound = errors.New("capture session not found")
	ErrCaptureNotFound = errors.New("capture not found")
	ErrStateConflict   = errors.New("capture state conflict")
)

type Repository interface {
	WithinTransaction(ctx context.Context, fn func(Repository) error) error
	CreateSession(ctx context.Context, session capturedomain.Session) error
	FindSession(ctx context.Context, id uuid.UUID) (*capturedomain.Session, error)
	FindCurrentSession(ctx context.Context, accountID uuid.UUID) (*capturedomain.Session, error)
	RevokeSessionsByAccount(ctx context.Context, accountID uuid.UUID, revokedAt time.Time) (int64, error)
	ClaimSession(ctx context.Context, pairingHash []byte, claim capturedomain.SessionClaim, desktopPresenceCutoff time.Time) (*capturedomain.Session, error)
	AuthenticateMobile(ctx context.Context, uploadTokenHash []byte, now time.Time) (*capturedomain.Session, error)
	AuthenticateUpload(ctx context.Context, uploadTokenHash []byte, now, desktopPresenceCutoff time.Time) (*capturedomain.Session, error)
	TouchDesktop(ctx context.Context, sessionID, accountID uuid.UUID, seenAt time.Time) error
	TouchMobile(ctx context.Context, sessionID uuid.UUID, uploadTokenHash []byte, seenAt time.Time) error
	RevokeSession(ctx context.Context, sessionID, accountID uuid.UUID, revokedAt time.Time) error

	CreateCapture(ctx context.Context, capture capturedomain.Capture) error
	SetCaptureAvailable(ctx context.Context, captureID uuid.UUID, storageURI string, updatedAt time.Time) (*capturedomain.Capture, error)
	MarkCaptureDeleting(ctx context.Context, captureID uuid.UUID, updatedAt time.Time) (*capturedomain.Capture, error)
	FindCapture(ctx context.Context, accountID, captureID uuid.UUID) (*capturedomain.Capture, error)
	ListCaptures(ctx context.Context, accountID uuid.UUID, now time.Time, page Pagination) ([]capturedomain.Capture, error)
	ListCleanupCandidates(ctx context.Context, now, uploadingCutoff time.Time, limit int) ([]capturedomain.Capture, error)
	DeleteCapture(ctx context.Context, captureID uuid.UUID) error
	DeleteExpiredSessions(ctx context.Context, now time.Time, limit int) (int64, error)
}

type Pagination struct {
	Limit  int
	Offset int
}

type FileStorage interface {
	ObjectURI(objectName string) (string, error)
	Upload(ctx context.Context, file io.Reader, objectName, contentType string) (string, error)
	Open(ctx context.Context, uri string) (io.ReadCloser, error)
	Delete(ctx context.Context, uri string) error
	GetSignedURL(ctx context.Context, uri string, expiresIn time.Duration) (string, error)
}
