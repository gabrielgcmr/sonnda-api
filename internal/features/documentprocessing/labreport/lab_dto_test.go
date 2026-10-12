// internal/features/documentprocessing/labreport/lab_dto_test.go
package labreport

import "testing"

func TestExtractedLabReportNormalize(t *testing.T) {
	empty := "   "
	patientName := "  Gabriel Carvalho  "
	testName := "  Glicose  "
	resultValue := "  99  "
	unit := " mg/dL "
	rawText := "  Resultado: 99 mg/dL  "

	report := &ExtractedLabReport{
		Metadata: ExtractionMetadata{
			Provider:  " document_ai ",
			Processor: " lab-parser ",
			Status:    ExtractionStatusSucceeded,
		},
		PatientName: &patientName,
		LabName:     &empty,
		Tests: []ExtractedTestResult{
			{
				TestName: testName,
				Material: &empty,
				Items: []ExtractedTestItem{
					{
						ParameterName: " Glicose ",
						ResultValue:   &resultValue,
						ResultUnit:    &unit,
						ReferenceText: &empty,
						RawText:       &rawText,
					},
				},
			},
		},
	}

	report.Normalize()

	if report.Metadata.Provider != "document_ai" {
		t.Fatalf("expected provider trimmed, got %q", report.Metadata.Provider)
	}
	if report.Metadata.Processor != "lab-parser" {
		t.Fatalf("expected processor trimmed, got %q", report.Metadata.Processor)
	}
	if report.PatientName == nil || *report.PatientName != "Gabriel Carvalho" {
		t.Fatalf("expected patient name trimmed, got %#v", report.PatientName)
	}
	if report.LabName != nil {
		t.Fatalf("expected empty lab name to become nil, got %#v", report.LabName)
	}
	if report.Tests[0].TestName != "Glicose" {
		t.Fatalf("expected test name trimmed, got %q", report.Tests[0].TestName)
	}
	if report.Tests[0].Material != nil {
		t.Fatalf("expected empty material to become nil, got %#v", report.Tests[0].Material)
	}

	item := report.Tests[0].Items[0]
	if item.ParameterName != "Glicose" {
		t.Fatalf("expected parameter name trimmed, got %q", item.ParameterName)
	}
	if item.ResultValue == nil || *item.ResultValue != "99" {
		t.Fatalf("expected result value trimmed, got %#v", item.ResultValue)
	}
	if item.ResultUnit == nil || *item.ResultUnit != "mg/dL" {
		t.Fatalf("expected result unit trimmed, got %#v", item.ResultUnit)
	}
	if item.ReferenceText != nil {
		t.Fatalf("expected empty reference to become nil, got %#v", item.ReferenceText)
	}
	if item.RawText == nil || *item.RawText != "Resultado: 99 mg/dL" {
		t.Fatalf("expected raw text trimmed, got %#v", item.RawText)
	}
}

func TestExtractedLabReportHasStructuredResults(t *testing.T) {
	tests := []struct {
		name string
		in   *ExtractedLabReport
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "empty", in: &ExtractedLabReport{}, want: false},
		{
			name: "missing test name",
			in: &ExtractedLabReport{
				Tests: []ExtractedTestResult{
					{Items: []ExtractedTestItem{{ParameterName: "Glicose"}}},
				},
			},
			want: false,
		},
		{
			name: "missing item name",
			in: &ExtractedLabReport{
				Tests: []ExtractedTestResult{
					{TestName: "Glicose", Items: []ExtractedTestItem{{ResultValue: stringPtr("99")}}},
				},
			},
			want: false,
		},
		{
			name: "structured",
			in: &ExtractedLabReport{
				Tests: []ExtractedTestResult{
					{TestName: "Glicose", Items: []ExtractedTestItem{{ParameterName: "Glicose"}}},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.HasStructuredResults(); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func stringPtr(value string) *string {
	return &value
}
