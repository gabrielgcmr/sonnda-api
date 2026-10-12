// internal/api/openapi_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

type expectedOperation struct {
	method, path, operationID, security string
}

func TestOpenAPIContainsExpectedOperationContract(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	bearer := bearerAuthScheme
	captureToken := captureTokenAuthScheme
	expected := []expectedOperation{
		{http.MethodGet, "/healthz", "getHealth", ""},
		{http.MethodGet, "/me", "getCurrentAccount", bearer},
		{http.MethodPatch, "/me", "updateCurrentAccount", bearer},
		{http.MethodDelete, "/me", "deleteCurrentAccount", bearer},
		{http.MethodPost, "/me/professional-activation", "activateCurrentAccountAsProfessional", bearer},
		{http.MethodGet, "/me/patients", "listAccessiblePatients", bearer},
		{http.MethodPost, "/patients", "createPatient", bearer},
		{http.MethodGet, "/patients/{patientId}", "getPatient", bearer},
		{http.MethodPatch, "/patients/{patientId}", "updatePatient", bearer},
		{http.MethodDelete, "/patients/{patientId}", "deletePatient", bearer},
		{http.MethodPost, "/patients/{patientId}/problems", "createPatientProblem", bearer},
		{http.MethodGet, "/patients/{patientId}/problems", "listPatientProblems", bearer},
		{http.MethodGet, "/patients/{patientId}/problems/{problemId}", "getPatientProblem", bearer},
		{http.MethodPut, "/patients/{patientId}/problems/{problemId}", "editPatientProblem", bearer},
		{http.MethodGet, "/patients/{patientId}/problems/{problemId}/history", "listPatientProblemHistory", bearer},
		{http.MethodPut, "/patients/{patientId}/problems/{problemId}/classification", "classifyPatientProblem", bearer},
		{http.MethodPost, "/patients/{patientId}/problems/{problemId}/resolve", "resolvePatientProblem", bearer},
		{http.MethodPost, "/patients/{patientId}/problems/{problemId}/reopen", "reopenPatientProblem", bearer},
		{http.MethodPost, "/patients/{patientId}/problems/{problemId}/rectify", "rectifyPatientProblem", bearer},
		{http.MethodPost, "/patients/{patientId}/problems/{problemId}/merge", "mergePatientProblems", bearer},
		{http.MethodGet, "/patients/{patientId}/exam-documents", "listExamDocuments", bearer},
		{http.MethodPost, "/patients/{patientId}/exam-documents", "uploadExamDocument", bearer},
		{http.MethodGet, "/patients/{patientId}/exam-document-texts", "listExamDocumentTexts", bearer},
		{http.MethodGet, "/exam-documents/{documentId}", "getExamDocument", bearer},
		{http.MethodDelete, "/exam-documents/{documentId}", "discardExamDocument", bearer},
		{http.MethodGet, "/exam-documents/{documentId}/file", "getExamDocumentFile", bearer},
		{http.MethodGet, "/exam-documents/{documentId}/extraction", "getExamDocumentExtraction", bearer},
		{http.MethodPost, "/exam-documents/{documentId}/confirmation", "confirmExamDocument", bearer},
		{http.MethodPost, "/lab-extractions", "extractStandaloneLabReport", bearer},
		{http.MethodPost, "/lab-extractions/from-capture", "extractStandaloneLabReportFromCapture", bearer},
		{http.MethodPost, "/imaging-report-extractions", "extractImagingReport", bearer},
		{http.MethodPost, "/imaging-report-extractions/from-capture", "extractImagingReportFromCapture", bearer},
		{http.MethodGet, "/patients/{patientId}/lab-reports", "listPatientLabReports", bearer},
		{http.MethodGet, "/lab-reports/{labReportId}", "getLabReport", bearer},
		{http.MethodPost, "/capture-sessions/claim", "claimCaptureSession", ""},
		{http.MethodPost, "/capture-sessions/{sessionId}/mobile-heartbeat", "heartbeatMobileCaptureSession", captureToken},
		{http.MethodPost, "/captures", "uploadCapture", captureToken},
		{http.MethodPost, "/capture-sessions", "createCaptureSession", bearer},
		{http.MethodGet, "/capture-sessions/current", "getCurrentCaptureSession", bearer},
		{http.MethodPost, "/capture-sessions/{sessionId}/heartbeat", "heartbeatCaptureSession", bearer},
		{http.MethodDelete, "/capture-sessions/{sessionId}", "revokeCaptureSession", bearer},
		{http.MethodGet, "/captures", "listCaptures", bearer},
		{http.MethodGet, "/captures/{captureId}/file", "getCaptureFile", bearer},
		{http.MethodDelete, "/captures/{captureId}", "deleteCapture", bearer},
	}

	for _, want := range expected {
		operation := openAPIOperation(spec, want.method, want.path)
		if operation == nil {
			t.Errorf("operação ausente: %s %s", want.method, want.path)
			continue
		}
		if operation.OperationID != want.operationID {
			t.Errorf("%s %s OperationID = %q; esperado %q", want.method, want.path, operation.OperationID, want.operationID)
		}
		if got := operationSecurity(operation); got != want.security {
			t.Errorf("%s %s segurança = %q; esperado %q", want.method, want.path, got, want.security)
		}
	}
	if got := operationCount(spec); got != len(expected) {
		t.Fatalf("quantidade de operações = %d; esperado %d", got, len(expected))
	}
	if spec.Paths["/internal/jobs/cleanup-captures"] != nil {
		t.Fatal("rota interna de limpeza foi publicada no OpenAPI")
	}
}

func TestRuntimeAndDocumentationOpenAPIStayIdentical(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	runtimeAPI, err := SetupRoutes(router, completeTestDependencies())
	if err != nil {
		t.Fatal(err)
	}
	runtimeJSON, err := json.Marshal(runtimeAPI.OpenAPI())
	if err != nil {
		t.Fatal(err)
	}
	documentationJSON, err := json.Marshal(OpenAPI(APIInfo{}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(runtimeJSON, documentationJSON) {
		t.Fatal("as especificações de runtime e documentation-only são diferentes")
	}

	cleanupRegistered := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/internal/jobs/cleanup-captures" {
			cleanupRegistered = true
			break
		}
	}
	if !cleanupRegistered {
		t.Fatal("rota interna de limpeza não foi registrada no runtime")
	}
	if runtimeAPI.OpenAPI().Paths["/internal/jobs/cleanup-captures"] != nil {
		t.Fatal("rota interna de limpeza foi publicada na especificação de runtime")
	}
}

func openAPIOperation(spec *huma.OpenAPI, method, path string) *huma.Operation {
	item := spec.Paths[path]
	if item == nil {
		return nil
	}
	switch method {
	case http.MethodGet:
		return item.Get
	case http.MethodPost:
		return item.Post
	case http.MethodPut:
		return item.Put
	case http.MethodPatch:
		return item.Patch
	case http.MethodDelete:
		return item.Delete
	default:
		return nil
	}
}

func operationSecurity(operation *huma.Operation) string {
	if len(operation.Security) == 0 {
		return ""
	}
	if len(operation.Security) != 1 || len(operation.Security[0]) != 1 {
		return "<invalid>"
	}
	for name := range operation.Security[0] {
		return name
	}
	return ""
}

func operationCount(spec *huma.OpenAPI) int {
	count := 0
	for _, item := range spec.Paths {
		for _, operation := range []*huma.Operation{item.Get, item.Post, item.Put, item.Patch, item.Delete, item.Head, item.Options, item.Trace} {
			if operation != nil {
				count++
			}
		}
	}
	return count
}

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

func TestOpenAPIExposesOnlyCurrentAccountPatientList(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	patients := spec.Paths["/patients"]
	if patients == nil || patients.Post == nil {
		t.Fatalf("patient creation route is missing: %+v", patients)
	}
	if patients.Get != nil {
		t.Fatalf("global patient listing must not be public: %+v", patients.Get)
	}
	currentAccountPatients := spec.Paths["/me/patients"]
	if currentAccountPatients == nil || currentAccountPatients.Get == nil {
		t.Fatalf("current-account patient list is missing: %+v", currentAccountPatients)
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

func TestOpenAPIIncludesAuthenticatedCaptureSessionOperations(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	operations := []*huma.Operation{
		spec.Paths["/capture-sessions"].Post,
		spec.Paths["/capture-sessions/current"].Get,
		spec.Paths["/capture-sessions/{sessionId}/heartbeat"].Post,
		spec.Paths["/capture-sessions/{sessionId}"].Delete,
		spec.Paths["/captures"].Get,
		spec.Paths["/captures/{captureId}/file"].Get,
		spec.Paths["/captures/{captureId}"].Delete,
	}
	for _, operation := range operations {
		if operation == nil || len(operation.Security) != 1 {
			t.Fatalf("missing authenticated capture operation: %+v", operation)
		}
		if _, ok := operation.Security[0][bearerAuthScheme]; !ok {
			t.Fatal("missing bearer security")
		}
	}
	create := spec.Paths["/capture-sessions"].Post
	if create.Responses["201"] == nil {
		t.Fatal("capture session creation must document 201")
	}
	response := referencedSchema(spec, create.Responses["201"].Content["application/json"].Schema)
	if response.Properties["pairing_code"] == nil || response.Properties["session_id"] == nil ||
		response.Properties["pairing_code_hash"] != nil || response.Properties["upload_token_hash"] != nil {
		t.Fatalf("unexpected create response schema: %+v", response.Properties)
	}
	current := referencedSchema(spec, spec.Paths["/capture-sessions/current"].Get.Responses["200"].Content["application/json"].Schema)
	if current.Properties["pairing_code"] != nil || current.Properties["session_id"] == nil {
		t.Fatalf("unexpected current response schema: %+v", current.Properties)
	}
	list := referencedSchema(spec, spec.Paths["/captures"].Get.Responses["200"].Content["application/json"].Schema)
	items := referencedSchema(spec, list.Properties["items"].Items)
	if items.Properties["id"] == nil || items.Properties["expires_at"] == nil || items.Properties["storage_uri"] != nil {
		t.Fatalf("unexpected capture list item schema: %+v", items.Properties)
	}
}

func TestOpenAPIPublishesCaptureLabExtractionContract(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	fromCapture := spec.Paths["/lab-extractions/from-capture"].Post
	upload := spec.Paths["/lab-extractions"].Post
	request := referencedSchema(spec, fromCapture.RequestBody.Content["application/json"].Schema)
	if request.Properties["capture_id"] == nil || len(request.Required) != 1 || request.Required[0] != "capture_id" {
		t.Fatalf("unexpected capture extraction request: properties=%+v required=%v", request.Properties, request.Required)
	}
	got := fromCapture.Responses["200"].Content["application/json"].Schema.Ref
	want := upload.Responses["200"].Content["application/json"].Schema.Ref
	if got == "" || got != want {
		t.Fatalf("capture extraction response %q must match upload extraction response %q", got, want)
	}
}

func TestOpenAPISeparatesPublicClaimAndMobileCaptureSecurity(t *testing.T) {
	spec := OpenAPI(APIInfo{})
	claim := spec.Paths["/capture-sessions/claim"].Post
	if claim == nil || len(claim.Security) != 0 {
		t.Fatalf("claim must be public: %+v", claim)
	}
	for _, operation := range []*huma.Operation{
		spec.Paths["/capture-sessions/{sessionId}/mobile-heartbeat"].Post,
		spec.Paths["/captures"].Post,
	} {
		if operation == nil || len(operation.Security) != 1 {
			t.Fatalf("missing mobile capture operation: %+v", operation)
		}
		if _, ok := operation.Security[0][captureTokenAuthScheme]; !ok {
			t.Fatal("mobile route does not use capture token security")
		}
		if _, ok := operation.Security[0][bearerAuthScheme]; ok {
			t.Fatal("mobile route unexpectedly accepts Supabase bearer auth")
		}
	}
	scheme := spec.Components.SecuritySchemes[captureTokenAuthScheme]
	if scheme == nil || scheme.Type != "apiKey" || scheme.In != "header" || scheme.Name != "X-Capture-Token" {
		t.Fatalf("unexpected capture token scheme: %+v", scheme)
	}
}
