// internal/features/documentprocessing/imagingreport/schema.go
package imagingreport

import _ "embed"

//go:embed imaging_report.schema.json
var responseSchema string

func ResponseSchema() string { return responseSchema }
