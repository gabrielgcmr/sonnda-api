// internal/features/documentprocessing/labreport/schema_test.go
package labreport

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileResponseSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	var document any
	if err := json.Unmarshal([]byte(LabReportResponseSchema()), &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const location = "lab_report.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func readContractFixture(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExtractLabReportInput(t *testing.T) {
	for _, text := range []string{"", " \t\n\r", "\u00a0"} {
		if err := (ExtractLabReportInput{Text: text}).Validate(); !errors.Is(err, ErrEmptyInput) {
			t.Fatalf("expected empty text error, got %v", err)
		}
	}
	input := ExtractLabReportInput{Text: "  Glicose\n99 mg/dL\n"}
	original := input.Text
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	if input.Text != original {
		t.Fatal("input validation changed the original text")
	}
	media := ExtractLabReportInput{Media: &MediaInput{Data: []byte{0xff, 0xd8, 0xff}, MIMEType: "image/jpeg"}}
	if err := media.Validate(); err != nil {
		t.Fatalf("valid media rejected: %v", err)
	}
	if err := (ExtractLabReportInput{Text: "Glicose 90", Media: media.Media}).Validate(); !errors.Is(err, ErrAmbiguousInput) {
		t.Fatalf("expected ambiguous input error, got %v", err)
	}
	if err := (ExtractLabReportInput{Media: &MediaInput{Data: []byte{1}}}).Validate(); !errors.Is(err, ErrInvalidMediaType) {
		t.Fatalf("expected media type error, got %v", err)
	}
}

// Valida exemplos esperados; nao executa OCR, classificacao ou LLM.
func TestLabReportSchemaFixtures(t *testing.T) {
	schema := compileResponseSchema(t)
	var cases []struct {
		Name       string `json:"name"`
		Structured bool   `json:"structured"`
	}
	if err := json.Unmarshal(readContractFixture(t, "cases.json"), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no contract fixtures")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			input := ExtractLabReportInput{Text: string(readContractFixture(t, tc.Name+".input.txt"))}
			if err := input.Validate(); err != nil {
				t.Fatal(err)
			}
			data := readContractFixture(t, tc.Name+".expected.json")
			var expected any
			if err := json.Unmarshal(data, &expected); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(expected); err != nil {
				t.Fatal(err)
			}
			var report ExtractedLabReport
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if report.HasStructuredResults() != tc.Structured {
				t.Fatalf("expected structured=%v", tc.Structured)
			}
			// Dados do backend nao devem entrar na resposta exigida da LLM.
			report.RawText = &input.Text
			report.Metadata = ExtractionMetadata{Provider: "test", Status: ExtractionStatusSucceeded}
			for i := range report.Tests {
				report.Tests[i].RawText = &input.Text
				report.Tests[i].Status = ExtractionStatusSucceeded
				for j := range report.Tests[i].Items {
					report.Tests[i].Items[j].RawText = &input.Text
					report.Tests[i].Items[j].Status = ExtractionStatusSucceeded
				}
			}
			encoded, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var roundTrip any
			if err := json.Unmarshal(encoded, &roundTrip); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(expected, roundTrip) {
				t.Fatalf("DTO changed contract fields or values:\n%s", encoded)
			}
		})
	}
}

func TestLabReportSchemaRejectsInvalidResponses(t *testing.T) {
	schema := compileResponseSchema(t)
	firstTest := func(doc map[string]any) map[string]any {
		return doc["tests"].([]any)[0].(map[string]any)
	}
	firstItem := func(doc map[string]any) map[string]any {
		return firstTest(doc)["items"].([]any)[0].(map[string]any)
	}
	cases := map[string]func(map[string]any){
		"missing_tests":          func(doc map[string]any) { delete(doc, "tests") },
		"null_tests":             func(doc map[string]any) { doc["tests"] = nil },
		"wrong_tests_type":       func(doc map[string]any) { doc["tests"] = "Glicose" },
		"patient_id_from_model":  func(doc map[string]any) { doc["patient_id"] = "arbitrary-id" },
		"raw_text_from_model":    func(doc map[string]any) { doc["raw_text"] = "rewritten" },
		"metadata_from_model":    func(doc map[string]any) { doc["metadata"] = map[string]any{"status": "succeeded"} },
		"missing_optional_value": func(doc map[string]any) { delete(doc, "lab_name") },
		"blank_test_name":        func(doc map[string]any) { firstTest(doc)["test_name"] = " \n" },
		"empty_items":            func(doc map[string]any) { firstTest(doc)["items"] = []any{} },
		"null_items":             func(doc map[string]any) { firstTest(doc)["items"] = nil },
		"unknown_test_field":     func(doc map[string]any) { firstTest(doc)["diagnosis"] = "invented" },
		"missing_parameter":      func(doc map[string]any) { delete(firstItem(doc), "parameter_name") },
		"blank_parameter":        func(doc map[string]any) { firstItem(doc)["parameter_name"] = " " },
		"numeric_result":         func(doc map[string]any) { firstItem(doc)["result_value"] = 99.0 },
		"empty_result":           func(doc map[string]any) { firstItem(doc)["result_value"] = "" },
		"blank_unit":             func(doc map[string]any) { firstItem(doc)["result_unit"] = " " },
		"missing_unit":           func(doc map[string]any) { delete(firstItem(doc), "result_unit") },
		"canonical_name":         func(doc map[string]any) { firstItem(doc)["canonical_name"] = "glucose" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(readContractFixture(t, "glicose.expected.json"), &document); err != nil {
				t.Fatal(err)
			}
			mutate(document)
			if err := schema.Validate(document); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestLabReportSchemaAllowsMissingResultAsNull(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal(readContractFixture(t, "glicose.expected.json"), &document); err != nil {
		t.Fatal(err)
	}
	item := document["tests"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	item["result_value"] = nil
	if err := compileResponseSchema(t).Validate(document); err != nil {
		t.Fatal(err)
	}
}
