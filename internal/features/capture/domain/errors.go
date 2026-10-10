// internal/features/capture/domain/errors.go
package capturedomain

import "errors"

var (
	ErrInvalidID               = errors.New("invalid capture id")
	ErrInvalidAccountID        = errors.New("invalid capture account id")
	ErrInvalidSessionID        = errors.New("invalid capture session id")
	ErrInvalidHash             = errors.New("invalid capture credential hash")
	ErrInvalidTimestamp        = errors.New("invalid capture timestamp")
	ErrInvalidClaim            = errors.New("invalid capture session claim")
	ErrInvalidFilename         = errors.New("invalid capture filename")
	ErrInvalidMIMEType         = errors.New("invalid capture MIME type")
	ErrInvalidSize             = errors.New("invalid capture size")
	ErrInvalidStatus           = errors.New("invalid capture status")
	ErrInvalidStorageURI       = errors.New("invalid capture storage URI")
	ErrInvalidExpiration       = errors.New("invalid capture expiration")
	ErrInvalidStatusTransition = errors.New("invalid capture status transition")
)
