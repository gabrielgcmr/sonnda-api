// internal/features/documentprocessing/labreport/summary.go
package labreport

import (
	"fmt"
	"strings"
	"time"
)

func FormatSummary(report *ExtractedLabReport) string {
	if report == nil {
		return "Nenhum resultado estruturado encontrado.\n"
	}

	var builder strings.Builder
	headerWritten := false
	if report.PatientName != nil {
		writeSummaryLine(&builder, "Paciente: "+*report.PatientName)
		headerWritten = true
	}
	if report.LabName != nil {
		writeSummaryLine(&builder, "Laboratório: "+*report.LabName)
		headerWritten = true
	}

	collectionDates := uniqueCollectionDates(report.Tests)
	if len(collectionDates) > 0 {
		writeSummaryLine(&builder, "Data de coleta: "+strings.Join(collectionDates, ", "))
		headerWritten = true
	}

	wroteTest := false
	for _, test := range report.Tests {
		if strings.TrimSpace(test.TestName) == "" {
			continue
		}
		if headerWritten || wroteTest {
			builder.WriteByte('\n')
		}
		builder.WriteString(test.TestName)
		builder.WriteByte('\n')
		for _, item := range test.Items {
			if strings.TrimSpace(item.ParameterName) == "" {
				continue
			}
			value := "não identificado"
			if item.ResultValue != nil {
				value = *item.ResultValue
			}
			if item.ResultUnit != nil {
				value += " " + *item.ResultUnit
			}
			writeSummaryLine(&builder, fmt.Sprintf("%s: %s", item.ParameterName, value))
		}
		wroteTest = true
	}

	if !wroteTest {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("Nenhum resultado estruturado encontrado.\n")
	}
	return builder.String()
}

func uniqueCollectionDates(tests []ExtractedTestResult) []string {
	seen := make(map[string]struct{})
	var dates []string
	for _, test := range tests {
		if test.CollectedAt == nil {
			continue
		}
		date := strings.TrimSpace(*test.CollectedAt)
		if date == "" {
			continue
		}
		date = formatCollectionDate(date)
		if _, exists := seen[date]; exists {
			continue
		}
		seen[date] = struct{}{}
		dates = append(dates, date)
	}
	return dates
}

func formatCollectionDate(value string) string {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			if layout == "2006-01-02" {
				return parsed.Format("02/01/2006")
			}
			return parsed.Format("02/01/2006 15:04")
		}
	}
	return value
}

func writeSummaryLine(builder *strings.Builder, line string) {
	builder.WriteString(line)
	builder.WriteByte('\n')
}
