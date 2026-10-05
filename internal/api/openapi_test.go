// internal/api/openapi_test.go
package api

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

func TestOpenAPIIncludesProfessionalActivation(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	path := spec.Paths["/me/professional-activation"]
	if path == nil || path.Post == nil || path.Post.OperationID != "activateCurrentAccountAsProfessional" {
		t.Fatalf("professional activation operation missing: %+v", path)
	}
}

func TestOpenAPIIncludesAuthenticatedProblemOperations(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	base := "/patients/{patientId}/problems"
	for _, path := range []string{base, base + "/{problemId}", base + "/{problemId}/history"} {
		if spec.Paths[path] == nil {
			t.Fatalf("missing problem path: %s", path)
		}
	}
	operations := []*huma.Operation{spec.Paths[base].Post, spec.Paths[base].Get, spec.Paths[base+"/{problemId}"].Get, spec.Paths[base+"/{problemId}/history"].Get}
	for _, operation := range operations {
		if operation == nil || len(operation.Security) != 1 {
			t.Fatalf("missing authenticated operation: %+v", operation)
		}
		if _, ok := operation.Security[0][bearerAuthScheme]; !ok {
			t.Fatal("missing bearer security")
		}
	}
	if spec.Paths[base].Post.Responses["201"] == nil {
		t.Fatal("creation must document 201")
	}
}
