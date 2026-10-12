// internal/features/documentprocessing/labreport/schema.go
package labreport

import _ "embed"

const LabReportSchemaVersion = "1"

//go:embed lab_report.schema.json
var labReportResponseSchema string

// LabReportResponseSchema compartilha o contrato entre testes e futuro adaptador.
func LabReportResponseSchema() string {
	return labReportResponseSchema
}
