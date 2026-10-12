// internal/application/usecase/labdocumentconfirmation/mapping.go
package labdocumentconfirmation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	labsvc "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory"
	labs "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/domain"
	"github.com/google/uuid"
)

func mapExtractedToDomain(
	patientID uuid.UUID,
	uploadedByUserID uuid.UUID,
	extracted *labreport.ExtractedLabReport,
) (*labs.LabReport, error) {
	if extracted == nil {
		return nil, labs.ErrInvalidInput
	}
	extracted.Normalize()

	report, err := labs.NewLabReport(patientID.String(), uploadedByUserID.String())
	if err != nil {
		return nil, err
	}

	report.PatientName = extracted.PatientName
	report.LabName = extracted.LabName
	report.LabPhone = extracted.LabPhone
	report.InsuranceProvider = extracted.InsuranceProvider
	report.RequestingDoctor = extracted.RequestingDoctor
	report.TechnicalManager = extracted.TechnicalManager

	if extracted.PatientDOB != nil {
		if t, err := parseDate(*extracted.PatientDOB); err == nil {
			report.PatientDOB = &t
		}
	}
	if extracted.ReportDate != nil {
		if t, err := parseDate(*extracted.ReportDate); err == nil {
			report.ReportDate = &t
		}
	}

	for _, et := range extracted.Tests {
		testResult, err := labs.NewLabPanel(report.ID.String(), et.TestName)
		if err != nil {
			return nil, err
		}

		testResult.Material = et.Material
		testResult.Method = et.Method

		if et.CollectedAt != nil {
			if t, err := parseDateTime(*et.CollectedAt); err == nil {
				testResult.CollectedAt = &t
			}
		}
		if et.ReleaseAt != nil {
			if t, err := parseDateTime(*et.ReleaseAt); err == nil {
				testResult.ReleaseAt = &t
			}
		}

		for _, ei := range et.Items {
			item, err := labs.NewObservation(testResult.ID.String(), ei.ParameterName)
			if err != nil {
				return nil, err
			}
			item.ResultValue = ei.ResultValue
			item.ResultUnit = ei.ResultUnit
			item.ReferenceText = ei.ReferenceText
			item.Normalize()
			testResult.Items = append(testResult.Items, *item)
		}

		testResult.Normalize()
		report.TestResults = append(report.TestResults, *testResult)
	}

	report.Normalize()
	report.UpdatedAt = time.Now().UTC()

	return report, nil
}

func parseDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)

	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02",
		"02/01/2006",
		"2006/01/02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}

	return time.Time{}, labs.ErrInvalidDateFormat
}

func parseDateTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "\u00e0s", " ")
	raw = strings.ReplaceAll(raw, " as ", " ")
	raw = strings.ReplaceAll(raw, "h", ":")
	raw = strings.Join(strings.Fields(raw), " ")

	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05-07:00",
		"02/01/2006 15:04:05",
		"02/01/2006 15:04",
		"02/01/2006 15:04:05 -0700",
		"02/01/2006 15:04 -0700",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}

	return parseDate(raw)
}

func normalize(s string) string {
	return strings.TrimSpace(strings.ToUpper(s))
}

func normalizeValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func generateLabFingerprint(patientID uuid.UUID, labReport *labs.LabReport) string {
	var parts []string
	patientKey := patientID.String()
	if patientKey == "" && labReport != nil {
		patientKey = labReport.PatientID.String()
	}

	for _, tr := range labReport.TestResults {
		var dateStr string
		if tr.CollectedAt != nil {
			dateStr = tr.CollectedAt.Format("2006-01-02")
		} else if labReport.ReportDate != nil {
			dateStr = labReport.ReportDate.Format("2006-01-02")
		} else {
			dateStr = "000-00-00"
		}

		testName := normalize(tr.TestName)
		for _, item := range tr.Items {
			param := normalize(item.ParameterName)
			value := normalizeValue(item.ResultValue)

			parts = append(parts,
				fmt.Sprintf("%s|%s|%s|%s|%s", patientKey, dateStr, testName, param, value),
			)
		}
	}

	sort.Strings(parts)

	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part))
	}

	hashBytes := hash.Sum(nil)
	return hex.EncodeToString(hashBytes)
}

func toOutput(report *labs.LabReport) *labsvc.LabReportOutput {
	output := &labsvc.LabReportOutput{
		ID:                report.ID,
		PatientID:         report.PatientID,
		ExamDocumentID:    report.ExamDocumentID,
		PatientName:       report.PatientName,
		PatientDOB:        report.PatientDOB,
		LabName:           report.LabName,
		LabPhone:          report.LabPhone,
		InsuranceProvider: report.InsuranceProvider,
		RequestingDoctor:  report.RequestingDoctor,
		TechnicalManager:  report.TechnicalManager,
		ReportDate:        report.ReportDate,
		UploadedByUserID:  report.UploadedBy,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
	}

	for _, tr := range report.TestResults {
		panelOutput := labsvc.LabPanelOutput{
			ID:          tr.ID,
			TestName:    tr.TestName,
			Material:    tr.Material,
			Method:      tr.Method,
			CollectedAt: tr.CollectedAt,
			ReleaseAt:   tr.ReleaseAt,
		}

		for _, item := range tr.Items {
			panelOutput.Observations = append(panelOutput.Observations, labsvc.ObservationOutput{
				ID:            item.ID,
				ParameterName: item.ParameterName,
				ResultValue:   item.ResultValue,
				ResultUnit:    item.ResultUnit,
				ReferenceText: item.ReferenceText,
			})
		}

		output.Panels = append(output.Panels, panelOutput)
	}

	return output
}
