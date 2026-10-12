// internal/infrastructure/gemini/lab_report_schema.go
package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var supportedProviderSchemaKeys = map[string]struct{}{
	"$id":                  {},
	"$defs":                {},
	"$ref":                 {},
	"$anchor":              {},
	"type":                 {},
	"format":               {},
	"title":                {},
	"description":          {},
	"enum":                 {},
	"items":                {},
	"prefixItems":          {},
	"minItems":             {},
	"maxItems":             {},
	"minimum":              {},
	"maximum":              {},
	"anyOf":                {},
	"oneOf":                {},
	"properties":           {},
	"additionalProperties": {},
	"required":             {},
	"propertyOrdering":     {},
}

func loadLabReportSchemas() (map[string]any, *jsonschema.Schema, error) {
	var document map[string]any
	if err := json.Unmarshal([]byte(labreport.LabReportResponseSchema()), &document); err != nil {
		return nil, nil, fmt.Errorf("decode lab report schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	const location = "lab_report.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		return nil, nil, fmt.Errorf("register lab report schema: %w", err)
	}
	localSchema, err := compiler.Compile(location)
	if err != nil {
		return nil, nil, fmt.Errorf("compile lab report schema: %w", err)
	}
	providerSchema, ok := toProviderSchema(document, "").(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("provider lab report schema is invalid")
	}
	return providerSchema, localSchema, nil
}

func toProviderSchema(value any, parentKey string) any {
	switch typed := value.(type) {
	case map[string]any:
		next := make(map[string]any, len(typed))
		for key, child := range typed {
			if parentKey != "properties" {
				if _, ok := supportedProviderSchemaKeys[key]; !ok {
					continue
				}
			}
			if key == "$schema" {
				continue
			}
			next[key] = toProviderSchema(child, key)
		}
		return next
	case []any:
		next := make([]any, 0, len(typed))
		for _, child := range typed {
			next = append(next, toProviderSchema(child, parentKey))
		}
		return next
	default:
		return value
	}
}
