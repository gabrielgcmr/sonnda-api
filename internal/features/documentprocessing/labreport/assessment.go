// internal/features/documentprocessing/labreport/assessment.go
package labreport

import (
	"strings"
)

// assessStructure records the initial extraction state before normalization warnings.
// Preserve assessments supplied by other extractors, including their node metadata.
func assessStructure(report *ExtractedLabReport) {
	if report.Metadata.Status != "" {
		return
	}
	status := ExtractionStatusSucceeded
	if !report.HasStructuredResults() {
		status = ExtractionStatusNeedsReview
		report.Metadata.Warnings = append(report.Metadata.Warnings, ExtractionWarning{
			Code:    "no_structured_results",
			Message: "nenhum resultado laboratorial estruturado foi encontrado",
			Field:   "tests",
		})
	}
	report.Metadata.Status = status
	for i := range report.Tests {
		test := &report.Tests[i]
		if test.Status == "" {
			test.Status = status
		}
		for j := range test.Items {
			if test.Items[j].Status == "" {
				test.Items[j].Status = status
			}
		}
	}
}

func assessResults(report *ExtractedLabReport) {
	status := report.Metadata.Status
	if !Usable(report) {
		status = ExtractionStatusNeedsReview
		report.Metadata.Warnings = append(report.Metadata.Warnings, ExtractionWarning{Code: "no_usable_results", Message: "Nenhum resultado laboratorial utilizável foi encontrado."})
	} else {
		for _, test := range report.Tests {
			for _, item := range test.Items {
				if item.ResultValue == nil {
					report.Metadata.Warnings = append(report.Metadata.Warnings, ExtractionWarning{Code: "missing_value", Message: "Há parâmetros sem valor identificado.", Field: "tests.items.result_value"})
					break
				}
			}
		}
		if len(report.Metadata.Warnings) > 0 && status == ExtractionStatusSucceeded {
			status = ExtractionStatusPartial
		}
	}
	report.Metadata.Status = status
}

func Usable(report *ExtractedLabReport) bool {
	if report == nil || report.Metadata.Status == ExtractionStatusFailed {
		return false
	}
	for _, test := range report.Tests {
		if strings.TrimSpace(test.TestName) == "" {
			continue
		}
		for _, item := range test.Items {
			if strings.TrimSpace(item.ParameterName) != "" && item.ResultValue != nil && strings.TrimSpace(*item.ResultValue) != "" {
				return true
			}
		}
	}
	return false
}
