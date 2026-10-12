// internal/features/documentprocessing/labreport/service.go
package labreport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	text "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textreading"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

const maxDocumentBytes = 10 * 1024 * 1024

type Result struct {
	Status           ExtractionStatus    `json:"status"`
	Warnings         []ExtractionWarning `json:"warnings"`
	Report           ExtractedLabReport  `json:"report"`
	SummaryText      string              `json:"summary_text"`
	ProcessingMethod string              `json:"-"`
}

type Service struct {
	text text.Extractor
	lab  LabReportExtractor
}

func New(reader text.Extractor, lab LabReportExtractor) *Service {
	return &Service{text: reader, lab: lab}
}

func unavailable(value any) bool {
	return value == nil || (reflect.ValueOf(value).Kind() == reflect.Ptr && reflect.ValueOf(value).IsNil())
}

func (s *Service) ExtractPDF(ctx context.Context, path, filename string) (*Result, error) {
	return s.ExtractDocument(ctx, path, filename, "application/pdf")
}

func (s *Service) ExtractDocument(ctx context.Context, path, filename, mimeType string) (*Result, error) {
	if s == nil || unavailable(s.lab) {
		return nil, apperr.Internal("A extração laboratorial está indisponível.", nil)
	}
	switch mimeType {
	case "application/pdf":
		if unavailable(s.text) {
			return nil, apperr.Internal("A leitura de PDF está indisponível.", nil)
		}
		return s.extractPDF(ctx, path, filename)
	case "image/jpeg", "image/png":
		return s.extractMedia(ctx, path, mimeType)
	default:
		return nil, &apperr.AppError{Kind: apperr.UNSUPPORTED_MEDIA_TYPE, Message: "Envie um arquivo PDF, JPEG ou PNG."}
	}
}

func (s *Service) extractPDF(ctx context.Context, path, filename string) (*Result, error) {
	extracted, err := s.text.Extract(ctx, text.ExtractInput{LocalPath: path, OriginalFilename: filename, MimeType: "application/pdf"})
	if err != nil {
		if errors.Is(err, text.ErrUnreadablePDF) {
			return s.extractMedia(ctx, path, "application/pdf")
		}
		return nil, technicalError("Não foi possível ler o PDF.", err)
	}
	if extracted == nil || strings.TrimSpace(extracted.Text) == "" {
		return s.extractMedia(ctx, path, "application/pdf")
	}
	result, err := s.ExtractText(ctx, extracted.Text)
	if err == nil {
		result.ProcessingMethod = extracted.Method
	}
	return result, err
}

func (s *Service) extractMedia(ctx context.Context, path, mimeType string) (*Result, error) {
	data, err := readDocument(path, maxDocumentBytes)
	if err != nil {
		return nil, err
	}
	report, err := s.lab.ExtractLabReport(ctx, ExtractLabReportInput{
		Media: &MediaInput{Data: data, MIMEType: mimeType},
	})
	if err != nil {
		return nil, technicalError("Não foi possível extrair os dados laboratoriais.", err)
	}
	return finalizeReport(report, "gemini_multimodal", nil)
}

func (s *Service) ExtractText(ctx context.Context, input string) (*Result, error) {
	if s == nil || unavailable(s.lab) {
		return nil, apperr.Internal("A extração laboratorial está indisponível.", nil)
	}
	if strings.TrimSpace(input) == "" {
		return nil, apperr.DomainRuleViolation("O documento não contém texto legível.", apperr.Violation{Field: "file", Reason: "text_not_readable"})
	}
	semanticText := text.NormalizeForSemanticExtraction(input)
	report, err := s.lab.ExtractLabReport(ctx, ExtractLabReportInput{Text: semanticText})
	if err != nil {
		return nil, technicalError("Não foi possível extrair os dados laboratoriais.", err)
	}
	return finalizeReport(report, "", &input)
}

func finalizeReport(report *ExtractedLabReport, processor string, rawText *string) (*Result, error) {
	if report == nil {
		return nil, apperr.Internal("O extrator não retornou um resultado.", nil)
	}
	report.Normalize()
	report.RawText = rawText
	assessStructure(report)
	normalizeDates(report)
	normalizeResults(report)
	assessResults(report)
	warnings := report.Metadata.Warnings
	if warnings == nil {
		warnings = []ExtractionWarning{}
	}
	return &Result{Status: report.Metadata.Status, Warnings: warnings, Report: *report, SummaryText: FormatSummary(report), ProcessingMethod: processor}, nil
}

func readDocument(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, apperr.Internal("Não foi possível abrir o documento.", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, apperr.Internal("Não foi possível ler o documento.", err)
	}
	if int64(len(data)) > maximum {
		return nil, &apperr.AppError{Kind: apperr.UPLOAD_SIZE_EXCEEDED, Message: fmt.Sprintf("o arquivo deve ter no máximo %d MiB", maximum/(1024*1024))}
	}
	if len(data) == 0 {
		return nil, apperr.Validation("arquivo vazio", apperr.Violation{Field: "file", Reason: "empty"})
	}
	return data, nil
}

func technicalError(message string, err error) error {
	kind := apperr.INFRA_EXTERNAL_SERVICE_ERROR
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		kind = apperr.INFRA_TIMEOUT
	}
	return &apperr.AppError{Kind: kind, Message: message, Cause: err}
}
