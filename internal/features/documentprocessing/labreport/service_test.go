// internal/features/documentprocessing/labreport/service_test.go
package labreport

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

type readerStub struct {
	output *text.ExtractOutput
	err    error
	calls  int
}

func (r *readerStub) Extract(context.Context, text.ExtractInput) (*text.ExtractOutput, error) {
	r.calls++
	return r.output, r.err
}

type labStub struct {
	report *ExtractedLabReport
	err    error
	input  ExtractLabReportInput
}

func (s *labStub) ExtractLabReport(_ context.Context, input ExtractLabReportInput) (*ExtractedLabReport, error) {
	s.input = input
	return s.report, s.err
}

func sample() *ExtractedLabReport {
	value, unit, raw, date := "< 5", "mg/dL", "Glicose < 5", "2026-09-01"
	return &ExtractedLabReport{Metadata: ExtractionMetadata{Provider: "test"}, Tests: []ExtractedTestResult{{TestName: "Glicose", CollectedAt: &date, RawText: &raw, Items: []ExtractedTestItem{{ParameterName: "Glicose", ResultValue: &value, ResultUnit: &unit, RawText: &raw, Status: ExtractionStatusPartial, Warnings: []ExtractionWarning{{Code: "test", Message: "Conferir"}}}}}}}
}

func temporaryDocument(t *testing.T, name string, contents []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractionPreservesResultsAndPrivateMetadata(t *testing.T) {
	reader := &readerStub{output: &text.ExtractOutput{Text: "Glicose 90", Method: "pdf_text_raw"}}
	provider := &labStub{report: sample()}
	result, err := New(reader, provider).ExtractPDF(context.Background(), "test.pdf", "test.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.SummaryText, "Glicose: < 5 mg/dL") || !Usable(&result.Report) {
		t.Fatalf("bad summary: %+v", result)
	}
	if result.Report.Metadata.Provider != "test" || result.ProcessingMethod != "pdf_text_raw" {
		t.Fatalf("metadata lost: %+v", result.Report.Metadata)
	}
	if provider.input.Text != "Glicose 90" || provider.input.Media != nil {
		t.Fatalf("text path changed: %+v", provider.input)
	}
	test := result.Report.Tests[0]
	if test.RawText == nil || *test.RawText != "Glicose < 5" {
		t.Fatalf("test raw text lost: %+v", test)
	}
	if len(test.Items) != 1 || test.Items[0].Status != ExtractionStatusPartial {
		t.Fatalf("item status lost: %+v", test.Items)
	}
}

func TestUnreadablePDFUsesGeminiMediaFallback(t *testing.T) {
	contents := []byte("%PDF-1.4 scanned")
	path := temporaryDocument(t, "scanned.pdf", contents)
	reader := &readerStub{err: text.ErrUnreadablePDF}
	provider := &labStub{report: sample()}

	result, err := New(reader, provider).ExtractPDF(context.Background(), path, "scanned.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if provider.input.Media == nil || provider.input.Media.MIMEType != "application/pdf" || !reflect.DeepEqual(provider.input.Media.Data, contents) {
		t.Fatalf("PDF fallback did not preserve media: %+v", provider.input.Media)
	}
	if provider.input.Text != "" || result.Report.RawText != nil || result.ProcessingMethod != "gemini_multimodal" {
		t.Fatalf("unexpected fallback result: %+v input=%+v", result.Report.Metadata, provider.input)
	}
}

func TestImagesGoDirectlyToGeminiMedia(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mimeType string
		contents []byte
	}{
		{name: "jpeg", mimeType: "image/jpeg", contents: []byte{0xff, 0xd8, 0xff, 0x01}},
		{name: "png", mimeType: "image/png", contents: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &readerStub{}
			provider := &labStub{report: sample()}
			path := temporaryDocument(t, "exam", tc.contents)
			result, err := New(reader, provider).ExtractDocument(context.Background(), path, "exam", tc.mimeType)
			if err != nil {
				t.Fatal(err)
			}
			if reader.calls != 0 || provider.input.Media == nil || provider.input.Media.MIMEType != tc.mimeType || !reflect.DeepEqual(provider.input.Media.Data, tc.contents) {
				t.Fatalf("image path readerCalls=%d media=%+v", reader.calls, provider.input.Media)
			}
			if result.Report.RawText != nil || result.ProcessingMethod != "gemini_multimodal" {
				t.Fatalf("unexpected image result: %+v", result.Report.Metadata)
			}
		})
	}
}

func TestTechnicalPDFErrorDoesNotFallback(t *testing.T) {
	for _, tc := range []struct {
		err  error
		kind apperr.ErrorKind
	}{
		{errors.New("pdftotext missing"), apperr.INFRA_EXTERNAL_SERVICE_ERROR},
		{context.DeadlineExceeded, apperr.INFRA_TIMEOUT},
	} {
		provider := &labStub{report: sample()}
		_, err := New(&readerStub{err: tc.err}, provider).ExtractPDF(context.Background(), "x.pdf", "x.pdf")
		var app *apperr.AppError
		if !errors.As(err, &app) || app.Kind != tc.kind {
			t.Fatalf("wrong error: %v", err)
		}
		if provider.input.Media != nil || provider.input.Text != "" {
			t.Fatalf("technical error triggered fallback: %+v", provider.input)
		}
	}
}

func TestExtractionRejectsUnsupportedDocumentType(t *testing.T) {
	_, err := New(&readerStub{}, &labStub{}).ExtractDocument(context.Background(), "x.txt", "x.txt", "text/plain")
	if apperr.ErrorCodeOf(err) != apperr.UNSUPPORTED_MEDIA_TYPE {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPartialAndEmptyExtraction(t *testing.T) {
	report := sample()
	report.Tests[0].Items = append(report.Tests[0].Items, ExtractedTestItem{ParameterName: "Outro"})
	invalid := "impossible"
	report.Tests[0].CollectedAt = &invalid
	result, err := New(nil, &labStub{report: report}).ExtractText(context.Background(), "laudo")
	if err != nil || result.Status != ExtractionStatusPartial || len(result.Warnings) == 0 || result.Report.Tests[0].CollectedAt != nil {
		t.Fatalf("invalid partial result: %+v %v", result, err)
	}
	result, err = New(nil, &labStub{report: &ExtractedLabReport{}}).ExtractText(context.Background(), "pedido")
	if err != nil || Usable(&result.Report) || result.Status != ExtractionStatusNeedsReview {
		t.Fatalf("empty report accepted: %+v", result)
	}
}
