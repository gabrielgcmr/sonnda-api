// internal/features/documentprocessing/labreport/merge_pages.go
package labreport

import "strings"

// MergePages keeps every observed result, including repeated tests with different dates or values.
// Each input was extracted separately so its page origin is unambiguous.
func MergePages(pages []*Result) *Result {
	if len(pages) == 0 {
		return nil
	}
	merged := &Result{Status: ExtractionStatusSucceeded, Warnings: []ExtractionWarning{}, ProcessingMethod: "gemini_multimodal"}
	for index, page := range pages {
		if page == nil {
			continue
		}
		if index == 0 {
			merged.Report = page.Report
			merged.Report.Tests = nil
		} else {
			checkIdentity := func(field string, current, incoming *string) {
				if current != nil && incoming != nil && !strings.EqualFold(strings.TrimSpace(*current), strings.TrimSpace(*incoming)) {
					merged.Warnings = append(merged.Warnings, ExtractionWarning{Code: "identity_conflict", Message: "Identificação divergente entre as páginas; confira o documento.", Field: field})
					merged.Status = ExtractionStatusNeedsReview
				}
			}
			checkIdentity("patient_name", merged.Report.PatientName, page.Report.PatientName)
			checkIdentity("patient_dob", merged.Report.PatientDOB, page.Report.PatientDOB)
			if merged.Report.PatientName == nil {
				merged.Report.PatientName = page.Report.PatientName
			}
			if merged.Report.PatientDOB == nil {
				merged.Report.PatientDOB = page.Report.PatientDOB
			}
			if merged.Report.LabName == nil {
				merged.Report.LabName = page.Report.LabName
			}
			if merged.Report.ReportDate == nil {
				merged.Report.ReportDate = page.Report.ReportDate
			}
		}
		for _, test := range page.Report.Tests {
			test.SourcePage = index + 1
			merged.Report.Tests = append(merged.Report.Tests, test)
		}
		merged.Warnings = append(merged.Warnings, page.Warnings...)
		if page.Status != ExtractionStatusSucceeded && merged.Status == ExtractionStatusSucceeded {
			merged.Status = page.Status
		}
	}
	merged.SummaryText = FormatSummary(&merged.Report)
	return merged
}
