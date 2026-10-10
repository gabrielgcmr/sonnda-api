// internal/api/humaspec/endpoint.go
package humaspec

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

// NotImplemented is a typed endpoint used only while Huma builds the OpenAPI document.
// Specification endpoints are never executed by the OpenAPI export path.
func NotImplemented[I, O any](context.Context, *I) (*O, error) {
	return nil, huma.Error501NotImplemented("endpoint disponível somente em execução")
}
