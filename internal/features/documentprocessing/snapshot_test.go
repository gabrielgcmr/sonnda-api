// internal/features/documentprocessing/snapshot_test.go
package documentprocessing

import (
	"reflect"
	"testing"

	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
)

func snapshotPtr[T any](v T) *T { return &v }

func TestSnapshotEncodeDecodeRoundtrip(t *testing.T) {
	warnings := []labreport.ExtractionWarning{{Code: "review", Message: "Check original", Field: "tests.items"}}
	raw := "Raw document text"
	val := "95"
	unit := "mg/dL"
	ref := "70-99"
	date := "2026-09-01"

	original := &labreport.Result{
		Status:      labreport.ExtractionStatusPartial,
		Warnings:    warnings,
		SummaryText: "Glicose: 95 mg/dL",
		Report: labreport.ExtractedLabReport{
			Metadata: labreport.ExtractionMetadata{
				Provider:   "gemini",
				Processor:  "test-processor",
				Model:      "gemini-1.5",
				Status:     labreport.ExtractionStatusPartial,
				Confidence: snapshotPtr(0.92),
				Warnings:   warnings,
			},
			RawText: &raw,
			Tests: []labreport.ExtractedTestResult{
				{
					TestName:    "Glicose",
					CollectedAt: &date,
					RawText:     snapshotPtr("Glicose em jejum"),
					Status:      labreport.ExtractionStatusPartial,
					Confidence:  snapshotPtr(0.9),
					Warnings:    warnings,
					Items: []labreport.ExtractedTestItem{
						{
							ParameterName: "Glicose",
							ResultValue:   &val,
							ResultUnit:    &unit,
							ReferenceText: &ref,
							RawText:       snapshotPtr("Glicose 95 mg/dL"),
							Status:        labreport.ExtractionStatusPartial,
							Confidence:    snapshotPtr(0.85),
							Warnings:      warnings,
						},
					},
				},
			},
		},
	}

	data, err := EncodeExtractionSnapshot(original)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	decoded, err := DecodeExtractionSnapshot(data)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("roundtrip mismatch:\ngot:  %#v\nwant: %#v", decoded, original)
	}
}

func TestDecodeExtractionSnapshotValidations(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		_, err := DecodeExtractionSnapshot([]byte("not json"))
		if err == nil {
			t.Fatal("expected error on invalid json")
		}
	})

	t.Run("unsupported version", func(t *testing.T) {
		_, err := DecodeExtractionSnapshot([]byte(`{"version": 2}`))
		if err == nil {
			t.Fatal("expected error on unsupported version")
		}
	})

	t.Run("mismatched tests count", func(t *testing.T) {
		payload := `{"version": 1, "result": {"report": {"tests": []}}, "tests_metadata": [{}]}`
		_, err := DecodeExtractionSnapshot([]byte(payload))
		if err == nil {
			t.Fatal("expected error on mismatched tests count")
		}
	})

	t.Run("mismatched items count", func(t *testing.T) {
		payload := `{"version": 1, "result": {"report": {"tests": [{"items": []}]}}, "tests_metadata": [{"items": [{}]}]}`
		_, err := DecodeExtractionSnapshot([]byte(payload))
		if err == nil {
			t.Fatal("expected error on mismatched items count")
		}
	})
}
