// internal/features/capture/domain/capture.go
package capturedomain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MaxFileSizeBytes = 10 * 1024 * 1024
	RetentionPeriod  = 24 * time.Hour
)

type Status string

const (
	StatusUploading Status = "uploading"
	StatusAvailable Status = "available"
	StatusDeleting  Status = "deleting"
)

type Capture struct {
	ID               uuid.UUID
	AccountID        uuid.UUID
	CaptureSessionID uuid.UUID
	StorageURI       *string
	OriginalFilename string
	MIMEType         string
	SizeBytes        int64
	Status           Status
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type NewCaptureParams struct {
	AccountID        uuid.UUID
	CaptureSessionID uuid.UUID
	OriginalFilename string
	MIMEType         string
	SizeBytes        int64
	CreatedAt        time.Time
}

func NewCapture(params NewCaptureParams) (Capture, error) {
	createdAt := params.CreatedAt.UTC()
	capture := Capture{
		ID:               uuid.New(),
		AccountID:        params.AccountID,
		CaptureSessionID: params.CaptureSessionID,
		OriginalFilename: strings.TrimSpace(params.OriginalFilename),
		MIMEType:         strings.ToLower(strings.TrimSpace(params.MIMEType)),
		SizeBytes:        params.SizeBytes,
		Status:           StatusUploading,
		ExpiresAt:        createdAt.Add(RetentionPeriod),
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	}
	return capture, capture.Validate()
}

func (c Capture) Validate() error {
	if c.ID == uuid.Nil {
		return ErrInvalidID
	}
	if c.AccountID == uuid.Nil {
		return ErrInvalidAccountID
	}
	if c.CaptureSessionID == uuid.Nil {
		return ErrInvalidSessionID
	}
	if strings.TrimSpace(c.OriginalFilename) == "" {
		return ErrInvalidFilename
	}
	if !validMIMEType(c.MIMEType) {
		return ErrInvalidMIMEType
	}
	if c.SizeBytes <= 0 || c.SizeBytes > MaxFileSizeBytes {
		return ErrInvalidSize
	}
	if c.Status != StatusUploading && c.Status != StatusAvailable && c.Status != StatusDeleting {
		return ErrInvalidStatus
	}
	if c.Status == StatusAvailable && (c.StorageURI == nil || strings.TrimSpace(*c.StorageURI) == "") {
		return ErrInvalidStorageURI
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.Before(c.CreatedAt) {
		return ErrInvalidTimestamp
	}
	if !c.ExpiresAt.Equal(c.CreatedAt.Add(RetentionPeriod)) {
		return ErrInvalidExpiration
	}
	return nil
}

func (c Capture) MakeAvailable(storageURI string, updatedAt time.Time) (Capture, error) {
	when := updatedAt.UTC()
	uri := strings.TrimSpace(storageURI)
	if c.Status != StatusUploading || uri == "" || when.Before(c.UpdatedAt) {
		return Capture{}, ErrInvalidStatusTransition
	}
	next := c
	next.StorageURI = &uri
	next.Status = StatusAvailable
	next.UpdatedAt = when
	return next, next.Validate()
}

func (c Capture) MarkDeleting(updatedAt time.Time) (Capture, error) {
	when := updatedAt.UTC()
	if c.Status == StatusDeleting || when.Before(c.UpdatedAt) {
		return Capture{}, ErrInvalidStatusTransition
	}
	next := c
	next.Status = StatusDeleting
	next.UpdatedAt = when
	return next, next.Validate()
}

func validMIMEType(value string) bool {
	switch value {
	case "application/pdf", "image/jpeg", "image/png":
		return true
	default:
		return false
	}
}
