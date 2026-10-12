// internal/features/documentprocessing/imagingreport/service_test.go
package imagingreport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	text "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textreading"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type textReaderStub struct {
	output *text.ExtractOutput
	err    error
	calls  int
}

func (s *textReaderStub) Extract(_ context.Context, _ text.ExtractInput) (*text.ExtractOutput, error) {
	s.calls++
	return s.output, s.err
}

type imagingExtractorStub struct {
	document *ExtractedDocument
	err      error
	input    ExtractInput
}

func (s *imagingExtractorStub) ExtractImagingReport(_ context.Context, input ExtractInput) (*ExtractedDocument, error) {
	s.input = input
	return s.document, s.err
}

func imagingSample() *ExtractedDocument {
	location, unit := "mama esquerda", "mm"
	return &ExtractedDocument{
		Kind:        DocumentKindImagingReport,
		Readability: ReadabilityComplete,
		Report: Report{
			Modality:       ModalityMammography,
			FindingsText:   stringPointer("NÃ£o hÃ¡ nÃ³dulos suspeitos na mama direita."),
			ImpressionText: stringPointer("BI-RADS 2."),
			Classifications: []Classification{{
				System: "BI-RADS", Value: "2", SourceText: "Categoria BI-RADS 2",
			}},
			Measurements: []Measurement{{
				Name: "linfonodo", Location: &location, ValueText: "12 x 8", Unit: &unit,
				SourceText: "linfonodo na mama esquerda medindo 12 x 8 mm",
			}},
		},
	}
}

func stringPointer(value string) *string { return &value }

func temporaryImagingDocument(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractDocumentUsesTextPDFAndPreservesExplicitClinicalDetails(t *testing.T) {
	reader := &textReaderStub{output: &text.ExtractOutput{Text: "Laudo textual", Method: "pdf_text_raw"}}
	model := &imagingExtractorStub{document: imagingSample()}
	result, err := New(reader, model).ExtractDocument(t.Context(), "report.pdf", "report.pdf", "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || model.input.Text != "Laudo textual" || model.input.Media != nil {
		t.Fatalf("unexpected text path: reader=%d input=%+v", reader.calls, model.input)
	}
	if result.ProcessingMethod != "pdf_text_raw" || result.Report.Modality != ModalityMammography {
		t.Fatalf("result metadata was not preserved: %+v", result)
	}
	if result.Report.FindingsText == nil || !strings.Contains(*result.Report.FindingsText, "NÃ£o hÃ¡") || !strings.Contains(*result.Report.FindingsText, "direita") {
		t.Fatalf("negation or laterality lost: %+v", result.Report.FindingsText)
	}
	if got := result.Report.Classifications[0]; got.Value != "2" || got.SourceText != "Categoria BI-RADS 2" {
		t.Fatalf("explicit classification lost: %+v", got)
	}
	if got := result.Report.Measurements[0]; got.ValueText != "12 x 8" || got.Unit == nil || *got.Unit != "mm" || got.SourceText != "linfonodo na mama esquerda medindo 12 x 8 mm" {
		t.Fatalf("measurement or source text lost: %+v", got)
	}
	for _, phrase := range []string{"Categoria BI-RADS 2", "12 x 8 mm", "mama esquerda"} {
		if !strings.Contains(result.SummaryText, phrase) {
			t.Fatalf("summary omitted %q: %s", phrase, result.SummaryText)
		}
	}
}

func TestExtractDocumentUsesGeminiForScannedPDFAndImages(t *testing.T) {
	for _, tc := range []struct {
		name, mimeType string
		content        []byte
		readerErr      error
	}{
		{name: "scanned pdf", mimeType: "application/pdf", content: []byte("%PDF-1.7 scanned"), readerErr: text.ErrUnreadablePDF},
		{name: "jpeg", mimeType: "image/jpeg", content: []byte{0xff, 0xd8, 0xff, 0xe0}},
		{name: "png", mimeType: "image/png", content: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &textReaderStub{err: tc.readerErr}
			model := &imagingExtractorStub{document: imagingSample()}
			path := temporaryImagingDocument(t, "document", tc.content)
			result, err := New(reader, model).ExtractDocument(t.Context(), path, "document", tc.mimeType)
			if err != nil {
				t.Fatal(err)
			}
			if model.input.Media == nil || model.input.Media.MIMEType != tc.mimeType || !reflect.DeepEqual(model.input.Media.Data, tc.content) {
				t.Fatalf("media not forwarded: %+v", model.input.Media)
			}
			if result.ProcessingMethod != "gemini_multimodal" {
				t.Fatalf("unexpected processing method: %s", result.ProcessingMethod)
			}
			if tc.mimeType != "application/pdf" && reader.calls != 0 {
				t.Fatalf("image unexpectedly used text reader %d times", reader.calls)
			}
		})
	}
}

func TestExtractDocumentRejectsIncompatibleUnreadableAndMultipleReports(t *testing.T) {
	for _, kind := range []DocumentKind{DocumentKindIncompatible, DocumentKindUnreadable, DocumentKindMultipleReports, DocumentKindRadiologyImage} {
		t.Run(string(kind), func(t *testing.T) {
			model := &imagingExtractorStub{document: &ExtractedDocument{Kind: kind, Readability: ReadabilityComplete}}
			path := temporaryImagingDocument(t, "image.jpg", []byte{0xff, 0xd8, 0xff, 0xe0})
			_, err := New(nil, model).ExtractDocument(t.Context(), path, "image.jpg", "image/jpeg")
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Kind != apperr.DOMAIN_RULE_VIOLATION {
				t.Fatalf("expected document rejection, got %v", err)
			}
		})
	}
}

func TestExtractDocumentDoesNotFallbackAfterTechnicalPDFFailure(t *testing.T) {
	model := &imagingExtractorStub{document: imagingSample()}
	_, err := New(&textReaderStub{err: errors.New("pdftotext unavailable")}, model).ExtractDocument(t.Context(), "report.pdf", "report.pdf", "application/pdf")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Kind != apperr.INFRA_EXTERNAL_SERVICE_ERROR {
		t.Fatalf("expected infrastructure failure, got %v", err)
	}
	if model.input.Media != nil && len(model.input.Media.Data) > 0 {
		t.Fatal("technical reader failure triggered Gemini fallback")
	}
}

func TestExtractDocumentMapsProviderTimeout(t *testing.T) {
	model := &imagingExtractorStub{err: context.DeadlineExceeded}
	_, err := New(&textReaderStub{output: &text.ExtractOutput{Text: "Laudo"}}, model).ExtractDocument(t.Context(), "report.pdf", "report.pdf", "application/pdf")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Kind != apperr.INFRA_TIMEOUT {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestExtractImagePagesPreservesOrderAndRejectsMixedPDF(t *testing.T) {
	first := []byte{0xff, 0xd8, 0xff, 0xe0, 0x01}
	second := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x02}
	pages := []DocumentPage{
		{Path: temporaryImagingDocument(t, "page-1.jpg", first), Filename: "page-1.jpg", MIMEType: "image/jpeg"},
		{Path: temporaryImagingDocument(t, "page-2.png", second), Filename: "page-2.png", MIMEType: "image/png"},
	}
	model := &imagingExtractorStub{document: imagingSample()}
	result, err := New(nil, model).ExtractImagePages(t.Context(), pages)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProcessingMethod != "gemini_multimodal" || len(model.input.MediaParts) != 2 ||
		!reflect.DeepEqual(model.input.MediaParts[0].Data, first) || !reflect.DeepEqual(model.input.MediaParts[1].Data, second) {
		t.Fatalf("image order not preserved: %+v", model.input.MediaParts)
	}
	pages[1].MIMEType = "application/pdf"
	_, err = New(nil, model).ExtractImagePages(t.Context(), pages)
	if apperr.ErrorCodeOf(err) != apperr.UNSUPPORTED_MEDIA_TYPE {
		t.Fatalf("mixed PDF accepted: %v", err)
	}
}
