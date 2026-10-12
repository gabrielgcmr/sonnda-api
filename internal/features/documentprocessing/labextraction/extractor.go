// internal/features/documentprocessing/labextraction/extractor.go
package labextraction

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrEmptyInput       = errors.New("lab extraction requires text or media")
	ErrAmbiguousInput   = errors.New("lab extraction accepts either text or media")
	ErrInvalidMediaType = errors.New("lab extraction media type is required")
)

type MediaInput struct {
	Data     []byte
	MIMEType string
}

// ExtractLabReportInput receives semantic text or one document prepared by the application.
// The application retains the original file and patient context.
type ExtractLabReportInput struct {
	Text  string
	Media *MediaInput
}

func (input ExtractLabReportInput) Validate() error {
	hasText := strings.TrimSpace(input.Text) != ""
	hasMedia := input.Media != nil && len(input.Media.Data) > 0
	if !hasText && !hasMedia {
		return ErrEmptyInput
	}
	if hasText && hasMedia {
		return ErrAmbiguousInput
	}
	if hasMedia && strings.TrimSpace(input.Media.MIMEType) == "" {
		return ErrInvalidMediaType
	}
	return nil
}

// LabReportExtractor decodes structured data and reports provider metadata.
// Normalization and result assessment belong to the extraction application service.
type LabReportExtractor interface {
	ExtractLabReport(ctx context.Context, input ExtractLabReportInput) (*ExtractedLabReport, error)
}
