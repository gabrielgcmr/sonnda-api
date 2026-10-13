// internal/features/documentprocessing/labreport/merge_pages_test.go
package labreport

import (
	"strings"
	"testing"
)

func TestMergePagesKeepsDistinctValuesAndOrigin(t *testing.T) {
	first, second := "Ana Silva", "Outra Pessoa"
	oldValue, newValue := "90", "110"
	pages := []*Result{
		{Status: ExtractionStatusSucceeded, Report: ExtractedLabReport{PatientName: &first, Tests: []ExtractedTestResult{{TestName: "Glicose", CollectedAt: ptr("2026-10-01"), Items: []ExtractedTestItem{{ParameterName: "Glicose", ResultValue: &oldValue}}}}}},
		{Status: ExtractionStatusSucceeded, Report: ExtractedLabReport{PatientName: &second, Tests: []ExtractedTestResult{{TestName: "Glicose", CollectedAt: ptr("2026-10-02"), Items: []ExtractedTestItem{{ParameterName: "Glicose", ResultValue: &newValue}}}}}},
	}
	merged := MergePages(pages)
	if merged.Status != ExtractionStatusNeedsReview || len(merged.Warnings) == 0 {
		t.Fatalf("expected identity review, got %+v", merged)
	}
	if len(merged.Report.Tests) != 2 || merged.Report.Tests[0].SourcePage != 1 || merged.Report.Tests[1].SourcePage != 2 {
		t.Fatalf("page origin/results lost: %+v", merged.Report.Tests)
	}
	if !strings.Contains(merged.SummaryText, "90") || !strings.Contains(merged.SummaryText, "110") || !strings.Contains(merged.SummaryText, "página 2") {
		t.Fatalf("summary lost observations or origin: %q", merged.SummaryText)
	}
}

func ptr(value string) *string { return &value }
