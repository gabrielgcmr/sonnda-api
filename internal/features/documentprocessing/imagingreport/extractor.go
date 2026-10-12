// internal/features/documentprocessing/imagingreport/extractor.go
package imagingreport

import (
	"context"
	"errors"
	"strings"
)

type Modality string

const (
	ModalityUltrasound  Modality = "ultrasound"
	ModalityCT          Modality = "ct"
	ModalityMRI         Modality = "mri"
	ModalityMammography Modality = "mammography"
	ModalityXRay        Modality = "xray"
	ModalityOther       Modality = "other"
	ModalityUnknown     Modality = "unknown"
)

type DocumentKind string

const (
	DocumentKindImagingReport   DocumentKind = "imaging_report"
	DocumentKindRadiologyImage  DocumentKind = "radiology_image"
	DocumentKindIncompatible    DocumentKind = "incompatible"
	DocumentKindUnreadable      DocumentKind = "unreadable"
	DocumentKindMultipleReports DocumentKind = "multiple_reports"
)

type Readability string

const (
	ReadabilityComplete Readability = "complete"
	ReadabilityPartial  Readability = "partial"
)

var (
	ErrEmptyInput            = errors.New("imaging extraction requires text or media")
	ErrInvalidClassification = errors.New("invalid document classification")
)

type MediaInput struct {
	Data     []byte
	MIMEType string
}

type ExtractInput struct {
	Text  string
	Media *MediaInput
}

func (input ExtractInput) Validate() error {
	hasText := strings.TrimSpace(input.Text) != ""
	hasMedia := input.Media != nil && len(input.Media.Data) > 0 && strings.TrimSpace(input.Media.MIMEType) != ""
	if !hasText && !hasMedia {
		return ErrEmptyInput
	}
	if hasText == hasMedia {
		return ErrEmptyInput
	}
	return nil
}

type Classification struct {
	System     string `json:"system"`
	Value      string `json:"value"`
	SourceText string `json:"source_text"`
}

type Measurement struct {
	Name       string  `json:"name"`
	Location   *string `json:"location"`
	ValueText  string  `json:"value_text"`
	Unit       *string `json:"unit"`
	SourceText string  `json:"source_text"`
}

type Report struct {
	Modality        Modality         `json:"modality"`
	ExamName        *string          `json:"exam_name"`
	BodyRegion      *string          `json:"body_region"`
	ExamDate        *string          `json:"exam_date"`
	TechniqueText   *string          `json:"technique_text"`
	FindingsText    *string          `json:"findings_text"`
	ImpressionText  *string          `json:"impression_text"`
	Classifications []Classification `json:"classifications"`
	Measurements    []Measurement    `json:"measurements"`
}

type ExtractedDocument struct {
	Kind        DocumentKind
	Readability Readability
	Report      Report
}

type Extractor interface {
	ExtractImagingReport(context.Context, ExtractInput) (*ExtractedDocument, error)
}
