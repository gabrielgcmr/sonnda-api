// internal/features/documentprocessing/labreport/date.go
package labreport

import (
	"strings"
	"time"
)

func normalizeDates(report *ExtractedLabReport) {
	normalize := func(value **string, field string) {
		if *value == nil {
			return
		}
		raw := strings.TrimSpace(**value)
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "02/01/2006 15:04:05", "02/01/2006 15:04", "2006-01-02", "02/01/2006", "2006/01/02"} {
			if parsed, err := time.Parse(layout, raw); err == nil {
				formatted := parsed.Format(time.RFC3339)
				if len(raw) == 10 {
					formatted = parsed.Format("2006-01-02")
				}
				*value = &formatted
				return
			}
		}
		*value = nil
		report.Metadata.Warnings = append(report.Metadata.Warnings, ExtractionWarning{Code: "invalid_date", Message: "Uma data não pôde ser identificada e foi deixada em branco.", Field: field})
	}
	normalize(&report.PatientDOB, "patient_dob")
	normalize(&report.ReportDate, "report_date")
	for i := range report.Tests {
		normalize(&report.Tests[i].CollectedAt, "tests.collected_at")
		normalize(&report.Tests[i].ReleaseAt, "tests.release_at")
	}
}
