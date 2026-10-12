// internal/features/documentprocessing/extraction/assessment_test.go
package extraction

import (
	"context"
	"reflect"
	"testing"

	lab "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labextraction"
)

type assessmentExtractorFunc func(context.Context, lab.ExtractLabReportInput) (*lab.ExtractedLabReport, error)

func (f assessmentExtractorFunc) ExtractLabReport(ctx context.Context, input lab.ExtractLabReportInput) (*lab.ExtractedLabReport, error) {
	return f(ctx, input)
}

func TestExtractTextPreparesSemanticInputAndPreservesOriginal(t *testing.T) {
	const original = "  Hematócrito 43,8 ％ ﹪ ٪ ㌫ %\n"
	const semantic = "  Hematócrito 43,8 % % % % %\n"
	calls := 0
	service := New(nil, assessmentExtractorFunc(func(_ context.Context, input lab.ExtractLabReportInput) (*lab.ExtractedLabReport, error) {
		calls++
		if input.Text != semantic {
			t.Fatalf("semantic input = %q, want %q", input.Text, semantic)
		}
		return sample(), nil
	}))
	result, err := service.ExtractText(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Report.RawText == nil || *result.Report.RawText != original {
		t.Fatalf("original text or call count changed: calls=%d raw=%v", calls, result.Report.RawText)
	}
}

func TestExtractionPreservesExistingAssessmentMetadata(t *testing.T) {
	for _, tc := range []struct {
		inputStatus, outputStatus lab.ExtractionStatus
	}{
		{lab.ExtractionStatusPartial, lab.ExtractionStatusPartial},
		{lab.ExtractionStatusNeedsReview, lab.ExtractionStatusNeedsReview},
		{lab.ExtractionStatusFailed, lab.ExtractionStatusNeedsReview},
	} {
		t.Run(string(tc.inputStatus), func(t *testing.T) {
			report := sample()
			confidence := 0.7
			upstreamWarning := lab.ExtractionWarning{Code: "upstream", Message: "Conferir origem."}
			report.Metadata.Status = tc.inputStatus
			report.Metadata.Confidence = &confidence
			report.Metadata.Warnings = []lab.ExtractionWarning{upstreamWarning}
			report.Tests[0].Status = lab.ExtractionStatusNeedsReview
			report.Tests[0].Items[0].Confidence = &confidence
			wantTests := append([]lab.ExtractedTestResult(nil), report.Tests...)
			wantTests[0].Items = append([]lab.ExtractedTestItem(nil), report.Tests[0].Items...)
			wantWarnings := []lab.ExtractionWarning{upstreamWarning, report.Tests[0].Items[0].Warnings[0]}
			if tc.inputStatus == lab.ExtractionStatusFailed {
				wantWarnings = append(wantWarnings, lab.ExtractionWarning{Code: "no_usable_results", Message: "Nenhum resultado laboratorial utilizável foi encontrado."})
			}
			result, err := New(nil, &labStub{report: report}).ExtractText(context.Background(), "laudo")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != tc.outputStatus || !reflect.DeepEqual(result.Warnings, wantWarnings) {
				t.Fatalf("assessment changed: status=%s warnings=%+v", result.Status, result.Warnings)
			}
			if !reflect.DeepEqual(result.Report.Tests, wantTests) || result.Report.Metadata.Confidence == nil || *result.Report.Metadata.Confidence != confidence {
				t.Fatalf("existing node metadata or confidence changed: %+v", result.Report)
			}
		})
	}
}
