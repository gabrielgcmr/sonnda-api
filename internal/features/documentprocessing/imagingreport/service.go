// internal/features/documentprocessing/imagingreport/service.go
package imagingreport

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
const MaxImagePages = 5

type DocumentPage struct {
	Path     string
	Filename string
	MIMEType string
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ImagingReportResult struct {
	Status           string    `json:"status"`
	Warnings         []Warning `json:"warnings"`
	Report           Report    `json:"report"`
	SummaryText      string    `json:"summary_text"`
	ProcessingMethod string    `json:"-"`
}

type Service struct {
	text  text.Extractor
	model Extractor
}

func New(reader text.Extractor, model Extractor) *Service {
	return &Service{text: reader, model: model}
}

func isUnavailable(value any) bool {
	return value == nil || (reflect.ValueOf(value).Kind() == reflect.Ptr && reflect.ValueOf(value).IsNil())
}

func (s *Service) ExtractDocument(ctx context.Context, path, filename, mimeType string) (*ImagingReportResult, error) {
	if s == nil || isUnavailable(s.model) {
		return nil, apperr.Internal("A extração de laudos de imagem está indisponível.", nil)
	}
	var extracted *ExtractedDocument
	var method string
	switch mimeType {
	case "application/pdf":
		if isUnavailable(s.text) {
			return nil, apperr.Internal("A leitura de PDF está indisponível.", nil)
		}
		textOutput, err := s.text.Extract(ctx, text.ExtractInput{LocalPath: path, OriginalFilename: filename, MimeType: mimeType})
		if err != nil && !errors.Is(err, text.ErrUnreadablePDF) {
			return nil, extractionError("Não foi possível ler o PDF.", err)
		}
		if err == nil && textOutput != nil && strings.TrimSpace(textOutput.Text) != "" {
			extracted, err = s.model.ExtractImagingReport(ctx, ExtractInput{Text: textOutput.Text})
			method = textOutput.Method
		} else {
			data, readErr := readMedia(path)
			if readErr != nil {
				return nil, readErr
			}
			extracted, err = s.model.ExtractImagingReport(ctx, ExtractInput{Media: &MediaInput{Data: data, MIMEType: mimeType}})
			method = "gemini_multimodal"
		}
		if err != nil {
			return nil, extractionError("Não foi possível extrair o laudo de imagem.", err)
		}
	case "image/jpeg", "image/png":
		data, err := readMedia(path)
		if err != nil {
			return nil, err
		}
		extracted, err = s.model.ExtractImagingReport(ctx, ExtractInput{Media: &MediaInput{Data: data, MIMEType: mimeType}})
		if err != nil {
			return nil, extractionError("Não foi possível extrair o laudo de imagem.", err)
		}
		method = "gemini_multimodal"
	default:
		return nil, &apperr.AppError{Kind: apperr.UNSUPPORTED_MEDIA_TYPE, Message: "Envie um arquivo PDF, JPEG ou PNG contendo um laudo escrito."}
	}
	return finalize(extracted, method)
}

func (s *Service) ExtractImagePages(ctx context.Context, pages []DocumentPage) (*ImagingReportResult, error) {
	if s == nil || isUnavailable(s.model) {
		return nil, apperr.Internal("A extração de laudos de imagem está indisponível.", nil)
	}
	if len(pages) < 2 || len(pages) > MaxImagePages {
		return nil, apperr.Validation("Selecione de 2 a 5 imagens do mesmo laudo.", apperr.Violation{Field: "files", Reason: "invalid_page_count"})
	}
	input := ExtractInput{MediaParts: make([]MediaInput, 0, len(pages))}
	total := 0
	for _, page := range pages {
		if page.MIMEType != "image/jpeg" && page.MIMEType != "image/png" {
			return nil, &apperr.AppError{Kind: apperr.UNSUPPORTED_MEDIA_TYPE, Message: "Envie apenas imagens JPEG ou PNG para um laudo com várias páginas."}
		}
		data, err := readMedia(page.Path)
		if err != nil {
			return nil, err
		}
		total += len(data)
		if total > maxDocumentBytes {
			return nil, &apperr.AppError{Kind: apperr.UPLOAD_SIZE_EXCEEDED, Message: "As imagens devem ter no máximo 10 MiB no total."}
		}
		input.MediaParts = append(input.MediaParts, MediaInput{Data: data, MIMEType: page.MIMEType})
	}
	extracted, err := s.model.ExtractImagingReport(ctx, input)
	if err != nil {
		return nil, extractionError("Não foi possível extrair o laudo de imagem.", err)
	}
	return finalize(extracted, "gemini_multimodal")
}

func finalize(document *ExtractedDocument, method string) (*ImagingReportResult, error) {
	if document == nil {
		return nil, apperr.Internal("O extrator não retornou um resultado.", nil)
	}
	switch document.Kind {
	case DocumentKindRadiologyImage:
		return nil, apperr.DomainRuleViolation("Envie o laudo escrito do exame. Imagens radiológicas isoladas não são compatíveis.", apperr.Violation{Field: "file", Reason: "radiology_image_not_supported"})
	case DocumentKindIncompatible:
		return nil, apperr.DomainRuleViolation("O documento não parece ser um laudo escrito de exame de imagem.", apperr.Violation{Field: "file", Reason: "incompatible_document"})
	case DocumentKindUnreadable:
		return nil, apperr.DomainRuleViolation("Não foi possível ler o documento enviado.", apperr.Violation{Field: "file", Reason: "unreadable_document"})
	case DocumentKindMultipleReports:
		return nil, apperr.DomainRuleViolation("Envie um arquivo com apenas um laudo de exame.", apperr.Violation{Field: "file", Reason: "multiple_reports"})
	case DocumentKindImagingReport:
	default:
		return nil, apperr.Internal("O extrator retornou uma classificação inválida.", ErrInvalidClassification)
	}
	if !validModality(document.Report.Modality) {
		return nil, apperr.Internal("O extrator retornou uma modalidade inválida.", ErrInvalidClassification)
	}
	normalize(&document.Report)
	status := "succeeded"
	warnings := []Warning{}
	if document.Readability == ReadabilityPartial {
		status = "needs_review"
		warnings = append(warnings, Warning{Code: "partial_readability", Message: "O documento foi lido parcialmente. Revise os campos extraídos."})
	} else if document.Readability != ReadabilityComplete {
		return nil, apperr.Internal("O extrator retornou uma legibilidade inválida.", ErrInvalidClassification)
	}
	if document.Report.Modality == ModalityUnknown {
		status = "needs_review"
		warnings = append(warnings, Warning{Code: "unknown_modality", Message: "A modalidade do exame não pôde ser identificada com segurança."})
	}
	return &ImagingReportResult{Status: status, Warnings: warnings, Report: document.Report, SummaryText: summary(document.Report), ProcessingMethod: method}, nil
}

func validModality(modality Modality) bool {
	switch modality {
	case ModalityUltrasound, ModalityCT, ModalityMRI, ModalityMammography, ModalityXRay, ModalityOther, ModalityUnknown:
		return true
	default:
		return false
	}
}

func normalize(report *Report) {
	trim := func(value *string) {
		if value != nil {
			cleaned := strings.TrimSpace(*value)
			if cleaned == "" {
				*value = ""
			} else {
				*value = cleaned
			}
		}
	}
	trim(report.ExamName)
	trim(report.BodyRegion)
	trim(report.ExamDate)
	trim(report.TechniqueText)
	trim(report.FindingsText)
	trim(report.ImpressionText)
	if report.Classifications == nil {
		report.Classifications = []Classification{}
	}
	for index := range report.Classifications {
		item := &report.Classifications[index]
		item.System = strings.TrimSpace(item.System)
		item.Value = strings.TrimSpace(item.Value)
		item.SourceText = strings.TrimSpace(item.SourceText)
	}
	if report.Measurements == nil {
		report.Measurements = []Measurement{}
	}
	for index := range report.Measurements {
		item := &report.Measurements[index]
		item.Name = strings.TrimSpace(item.Name)
		item.ValueText = strings.TrimSpace(item.ValueText)
		item.SourceText = strings.TrimSpace(item.SourceText)
		trim(item.Location)
		trim(item.Unit)
	}
}

func summary(report Report) string {
	lines := make([]string, 0, 8+len(report.Classifications)+len(report.Measurements))
	add := func(label string, value *string) {
		if value != nil && strings.TrimSpace(*value) != "" {
			lines = append(lines, label+": "+strings.TrimSpace(*value))
		}
	}
	lines = append(lines, "Modalidade: "+modalityLabel(report.Modality))
	add("Exame", report.ExamName)
	add("Região anatômica", report.BodyRegion)
	add("Data", report.ExamDate)
	add("Técnica", report.TechniqueText)
	add("Achados", report.FindingsText)
	add("Conclusão", report.ImpressionText)
	for _, item := range report.Classifications {
		lines = append(lines, fmt.Sprintf("Classificação %s: %s (fonte: %s)", item.System, item.Value, item.SourceText))
	}
	for _, item := range report.Measurements {
		value := item.ValueText
		if item.Unit != nil && *item.Unit != "" {
			value += " " + *item.Unit
		}
		label := item.Name
		if item.Location != nil && *item.Location != "" {
			label += " — " + *item.Location
		}
		lines = append(lines, fmt.Sprintf("Medida %s: %s (fonte: %s)", label, value, item.SourceText))
	}
	return strings.Join(lines, "\n")
}

func modalityLabel(modality Modality) string {
	switch modality {
	case ModalityUltrasound:
		return "Ultrassonografia"
	case ModalityCT:
		return "Tomografia computadorizada"
	case ModalityMRI:
		return "Ressonância magnética"
	case ModalityMammography:
		return "Mamografia"
	case ModalityXRay:
		return "Radiografia"
	case ModalityOther:
		return "Outra"
	default:
		return "Não identificada"
	}
}

func readMedia(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, apperr.Internal("Não foi possível abrir o documento.", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return nil, apperr.Internal("Não foi possível ler o documento.", err)
	}
	if int64(len(data)) > maxDocumentBytes {
		return nil, &apperr.AppError{Kind: apperr.UPLOAD_SIZE_EXCEEDED, Message: "o arquivo deve ter no máximo 10 MiB"}
	}
	if len(data) == 0 {
		return nil, apperr.Validation("arquivo vazio", apperr.Violation{Field: "file", Reason: "empty"})
	}
	return data, nil
}

func extractionError(message string, err error) error {
	kind := apperr.INFRA_EXTERNAL_SERVICE_ERROR
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		kind = apperr.INFRA_TIMEOUT
	}
	return &apperr.AppError{Kind: kind, Message: message, Cause: err}
}
