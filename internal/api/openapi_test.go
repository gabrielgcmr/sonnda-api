// internal/api/openapi_test.go
package api

import "testing"

func TestOpenAPIIncludesProfessionalActivation(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	path := spec.Paths["/me/professional-activation"]
	if path == nil || path.Post == nil || path.Post.OperationID != "activateCurrentAccountAsProfessional" {
		t.Fatalf("professional activation operation missing: %+v", path)
	}
}
