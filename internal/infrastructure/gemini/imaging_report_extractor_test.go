// internal/infrastructure/gemini/imaging_report_extractor_test.go
package gemini

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"google.golang.org/genai"
)

const validImagingResponse = `{"document_kind":"imaging_report","readability":"complete","report":{"patient_name":"Maria Souza","modality":"mammography","exam_name":"Mamografia","body_region":"mamas","exam_date":null,"technique_text":null,"findings_text":"NÃ£o hÃ¡ nÃ³dulos suspeitos Ã  direita.","impression_text":"BI-RADS 2","classifications":[{"system":"BI-RADS","value":"2","source_text":"Categoria BI-RADS 2"}],"measurements":[{"name":"linfonodo","location":"mama esquerda","value_text":"12 x 8","unit":"mm","source_text":"linfonodo na mama esquerda medindo 12 x 8 mm"}]}}`

func TestImagingReportExtractorPreservesExplicitSourceAndMultimodalInput(t *testing.T) {
	media := []byte{0xff, 0xd8, 0xff, 0xe0}
	var captured GenerateRequest
	client := labReportExtractorTestClient(t, generatorFunc(func(_ context.Context, _ string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		captured = GenerateRequest{SystemInstruction: config.SystemInstruction.Parts[0].Text}
		if len(contents) != 1 || len(contents[0].Parts) != 1 || contents[0].Parts[0].InlineData == nil {
			t.Fatalf("unexpected multimodal content: %+v", contents)
		}
		captured.Media = &MediaInput{Data: contents[0].Parts[0].InlineData.Data, MIMEType: contents[0].Parts[0].InlineData.MIMEType}
		return geminiTextResponse(validImagingResponse), nil
	}))
	extractor, err := NewImagingReportExtractor(client)
	if err != nil {
		t.Fatal(err)
	}
	document, err := extractor.ExtractImagingReport(t.Context(), imagingreport.ExtractInput{Media: &imagingreport.MediaInput{Data: media, MIMEType: "image/jpeg"}})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Media == nil || captured.Media.MIMEType != "image/jpeg" || string(captured.Media.Data) != string(media) {
		t.Fatalf("media not forwarded: %+v", captured.Media)
	}
	if !strings.Contains(captured.SystemInstruction, "Preserve negações, lateralidade, valores e unidades") {
		t.Fatal("prompt does not require preserving clinical wording")
	}
	if document.Report.PatientName == nil || *document.Report.PatientName != "Maria Souza" {
		t.Fatalf("patient name lost: %+v", document.Report.PatientName)
	}
	if document.Report.FindingsText == nil || !strings.Contains(*document.Report.FindingsText, "NÃ£o hÃ¡") || !strings.Contains(*document.Report.FindingsText, "direita") {
		t.Fatalf("negation or laterality lost: %+v", document.Report.FindingsText)
	}
	if got := document.Report.Classifications[0]; got.Value != "2" || got.SourceText != "Categoria BI-RADS 2" {
		t.Fatalf("explicit classification lost: %+v", got)
	}
	if got := document.Report.Measurements[0]; got.ValueText != "12 x 8" || got.Unit == nil || *got.Unit != "mm" || got.SourceText != "linfonodo na mama esquerda medindo 12 x 8 mm" {
		t.Fatalf("explicit measurement lost: %+v", got)
	}
}

func TestImagingReportExtractorRejectsInvalidProviderResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		want    error
	}{
		{name: "invalid json", payload: "{"},
		{name: "invalid modality", payload: strings.Replace(validImagingResponse, `"mammography"`, `"unsupported"`, 1)},
		{name: "classification without source", payload: strings.Replace(validImagingResponse, `"source_text":"Categoria BI-RADS 2"`, `"source_text":" "`, 1)},
		{name: "measurement without source", payload: strings.Replace(validImagingResponse, `"source_text":"linfonodo na mama esquerda medindo 12 x 8 mm"`, `"source_text":" "`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := labReportExtractorTestClient(t, generatorFunc(func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
				return geminiTextResponse(tc.payload), nil
			}))
			extractor, err := NewImagingReportExtractor(client)
			if err != nil {
				t.Fatal(err)
			}
			_, err = extractor.ExtractImagingReport(t.Context(), imagingreport.ExtractInput{Text: "Laudo"})
			if err == nil {
				t.Fatal("invalid provider response was accepted")
			}
		})
	}
}

func TestImagingReportExtractorPropagatesProviderTimeout(t *testing.T) {
	client := labReportExtractorTestClient(t, generatorFunc(func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		return nil, context.DeadlineExceeded
	}))
	extractor, err := NewImagingReportExtractor(client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = extractor.ExtractImagingReport(t.Context(), imagingreport.ExtractInput{Text: "Laudo"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
}
