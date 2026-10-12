// internal/features/documentprocessing/extraction/characterization_test.go
package extraction_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/config"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	lab "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labextraction"
	text "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textextraction"
	"github.com/gabrielgcmr/sonnda/internal/infrastructure/gemini"
	"google.golang.org/genai"
)

type baselineReader struct {
	t     *testing.T
	calls int
}

const baselineText = "  Laudo sintético\nHematócrito 43,8 ％\n"

func (r *baselineReader) Extract(_ context.Context, input text.ExtractInput) (*text.ExtractOutput, error) {
	r.calls++
	want := text.ExtractInput{LocalPath: "synthetic.pdf", OriginalFilename: "synthetic.pdf", MimeType: "application/pdf"}
	if input != want {
		r.t.Fatalf("reader input = %+v, want %+v", input, want)
	}
	return &text.ExtractOutput{Text: baselineText, Method: "pdf_text_raw"}, nil
}

type baselineGenerator func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)

func (f baselineGenerator) GenerateContent(ctx context.Context, model string, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return f(ctx, model, contents, cfg)
}

type baselineLabExtractor func(context.Context, lab.ExtractLabReportInput) (*lab.ExtractedLabReport, error)

func (f baselineLabExtractor) ExtractLabReport(ctx context.Context, input lab.ExtractLabReportInput) (*lab.ExtractedLabReport, error) {
	return f(ctx, input)
}

func assertBaselineJSON(t *testing.T, got, want []byte) {
	t.Helper()
	var actual, expected any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("JSON changed\ngot:  %s\nwant: %s", got, want)
	}
}

func TestExtractionBehaviorBaseline(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "characterization.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name      string          `json:"name"`
		UseGemini bool            `json:"use_gemini"`
		Input     json.RawMessage `json:"input"`
		Expected  json.RawMessage `json:"expected"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("missing characterization cases")
	}
	// The same fixtures must hold without SDK or provider-specific behavior.
	for _, tc := range cases {
		if tc.UseGemini {
			tc.Name += "_without_provider"
			tc.UseGemini = false
			cases = append(cases, tc)
		}
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			calls := 0
			provider, model := "stub", "baseline-model"
			var extractor lab.LabReportExtractor = baselineLabExtractor(func(_ context.Context, input lab.ExtractLabReportInput) (*lab.ExtractedLabReport, error) {
				calls++
				if input.Text != strings.ReplaceAll(baselineText, "\uFF05", "%") {
					t.Fatalf("semantic extractor input changed: %q", input.Text)
				}
				var report lab.ExtractedLabReport
				if err := json.Unmarshal(tc.Input, &report); err != nil {
					t.Fatal(err)
				}
				report.Metadata = lab.ExtractionMetadata{Provider: provider, Model: model}
				return &report, nil
			})
			if tc.UseGemini {
				provider = "gemini"
				client, err := gemini.NewClientWithGenerator(config.GeminiConfig{
					Model: model, Timeout: time.Second, MaxInputBytes: 64 * 1024, MaxMediaBytes: 10 * 1024 * 1024, MaxOutputTokens: 4096,
				}, baselineGenerator(func(_ context.Context, gotModel string, contents []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
					calls++
					if gotModel != model || len(contents) != 1 || len(contents[0].Parts) != 1 {
						t.Fatalf("unexpected model or content: %s, %+v", gotModel, contents)
					}
					if got := contents[0].Parts[0].Text; got != strings.ReplaceAll(baselineText, "％", "%") {
						t.Fatalf("semantic input changed: %q", got)
					}
					return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
						Content: genai.NewContentFromText(string(tc.Input), genai.RoleModel), FinishReason: genai.FinishReasonStop,
					}}}, nil
				}))
				if err != nil {
					t.Fatal(err)
				}
				extractor, err = gemini.NewLabReportExtractor(client)
				if err != nil {
					t.Fatal(err)
				}
			}
			reader := &baselineReader{t: t}
			result, err := extraction.New(reader, extractor).ExtractPDF(context.Background(), "synthetic.pdf", "synthetic.pdf")
			if err != nil {
				t.Fatal(err)
			}
			if reader.calls != 1 || calls != 1 {
				t.Fatalf("expected one call per stage, reader=%d extractor=%d", reader.calls, calls)
			}
			public, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			assertBaselineJSON(t, public, tc.Expected)
			if result.Report.RawText == nil || *result.Report.RawText != baselineText {
				t.Fatalf("raw text changed: %v", result.Report.RawText)
			}
			var expected struct {
				Status   lab.ExtractionStatus    `json:"status"`
				Warnings []lab.ExtractionWarning `json:"warnings"`
			}
			if err := json.Unmarshal(tc.Expected, &expected); err != nil {
				t.Fatal(err)
			}
			// The public response uses [], while private metadata retains nil when there are no warnings.
			if len(expected.Warnings) == 0 {
				expected.Warnings = nil
			}
			wantMetadata := lab.ExtractionMetadata{Provider: provider, Model: model, Status: expected.Status, Warnings: expected.Warnings}
			if !reflect.DeepEqual(result.Report.Metadata, wantMetadata) {
				t.Fatalf("private metadata = %+v, want %+v", result.Report.Metadata, wantMetadata)
			}
			// The application records initial node success before partial-result warnings.
			nodeStatus := lab.ExtractionStatusSucceeded
			for _, test := range result.Report.Tests {
				if test.Status != nodeStatus || test.RawText != nil || test.Confidence != nil || test.Warnings != nil {
					t.Fatalf("test metadata changed: %+v", test)
				}
				for _, item := range test.Items {
					if item.Status != nodeStatus || item.RawText != nil || item.Confidence != nil || item.Warnings != nil {
						t.Fatalf("item metadata changed: %+v", item)
					}
				}
			}
		})
	}
}
