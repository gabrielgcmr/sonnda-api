// internal/api/openapi_test.go
package api

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

func TestOpenAPIUsesCurrentAccountPatchContract(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	path := spec.Paths["/me"]
	if path == nil || path.Get == nil || path.Patch == nil || path.Delete == nil {
		t.Fatalf("GET/PATCH/DELETE /me are required: %+v", path)
	}
	if path.Post != nil || path.Put != nil {
		t.Fatalf("legacy POST/PUT /me remain published: %+v", path)
	}

	requestSchema := referencedSchema(spec, path.Patch.RequestBody.Content["application/json"].Schema)
	if len(requestSchema.Required) != 0 {
		t.Fatalf("patch fields must be optional: %v", requestSchema.Required)
	}
	for _, name := range []string{"full_name", "birth_date", "cpf", "phone"} {
		property := requestSchema.Properties[name]
		if property == nil || !property.Nullable {
			t.Fatalf("patch property %q is not nullable: %+v", name, property)
		}
	}
	for _, name := range []string{"id", "email", "account_type", "onboarding_completed"} {
		if requestSchema.Properties[name] != nil {
			t.Fatalf("immutable property %q is writable", name)
		}
	}

	responseSchema := referencedSchema(spec, path.Get.Responses["200"].Content["application/json"].Schema)
	if responseSchema.Properties["profile"] == nil || responseSchema.Properties["onboarding_completed"] == nil ||
		responseSchema.Properties["auth_issuer"] != nil || responseSchema.Properties["full_name"] != nil {
		t.Fatalf("unexpected account response schema: %+v", responseSchema.Properties)
	}
}

func referencedSchema(spec *huma.OpenAPI, schema *huma.Schema) *huma.Schema {
	if schema.Ref == "" {
		return schema
	}
	return spec.Components.Schemas.SchemaFromRef(schema.Ref)
}

func TestOpenAPIIncludesProfessionalActivation(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	path := spec.Paths["/me/professional-activation"]
	if path == nil || path.Post == nil || path.Post.OperationID != "activateCurrentAccountAsProfessional" {
		t.Fatalf("professional activation operation missing: %+v", path)
	}
}

func TestOpenAPIKeepsBothPatientListRoutes(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	for _, route := range []string{"/patients", "/me/patients"} {
		path := spec.Paths[route]
		if path == nil || path.Get == nil {
			t.Fatalf("missing patient list route: %s", route)
		}
	}
}

func TestOpenAPIIncludesAuthenticatedProblemOperations(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	base := "/patients/{patientId}/problems"
	for _, path := range []string{base, base + "/{problemId}", base + "/{problemId}/history", base + "/{problemId}/rectify", base + "/{problemId}/merge"} {
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
	rectify := spec.Paths[base+"/{problemId}/rectify"].Post
	if rectify == nil || rectify.OperationID != "rectifyPatientProblem" || len(rectify.Security) != 1 {
		t.Fatalf("missing authenticated rectification operation: %+v", rectify)
	}
	request := referencedSchema(spec, rectify.RequestBody.Content["application/json"].Schema)
	if request.Properties["version"] == nil || request.Properties["reason"] == nil {
		t.Fatalf("unexpected rectification request: %+v", request.Properties)
	}
	required := map[string]bool{}
	for _, name := range request.Required {
		required[name] = true
	}
	if !required["version"] || !required["reason"] {
		t.Fatalf("rectification version and reason must be required: %v", request.Required)
	}
	merge := spec.Paths[base+"/{problemId}/merge"].Post
	if merge == nil || merge.OperationID != "mergePatientProblems" || len(merge.Security) != 1 {
		t.Fatalf("missing authenticated merge operation: %+v", merge)
	}
	mergeRequest := referencedSchema(spec, merge.RequestBody.Content["application/json"].Schema)
	mergeRequired := map[string]bool{}
	for _, name := range mergeRequest.Required {
		mergeRequired[name] = true
	}
	for _, name := range []string{"version", "sources", "name", "cid11", "classification", "clinical_status"} {
		if mergeRequest.Properties[name] == nil || !mergeRequired[name] {
			t.Fatalf("merge property %q must be present and required: properties=%+v required=%v", name, mergeRequest.Properties, mergeRequest.Required)
		}
	}
	if !mergeRequest.Properties["cid11"].Nullable {
		t.Fatalf("merge cid11 must accept explicit null: %+v", mergeRequest.Properties["cid11"])
	}
}
