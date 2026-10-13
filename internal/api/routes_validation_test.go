// internal/api/routes_validation_test.go
package api

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateAPIDependenciesReportsEveryMissingDependency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name  string
		clear func(*APIDependencies)
	}{
		{"Auth", func(deps *APIDependencies) { deps.Auth = nil }},
		{"Account", func(deps *APIDependencies) { deps.Account = nil }},
		{"AccountHandler", func(deps *APIDependencies) { deps.AccountHandler = nil }},
		{"CaptureHandler", func(deps *APIDependencies) { deps.CaptureHandler = nil }},
		{"CaptureAuth", func(deps *APIDependencies) { deps.CaptureAuth = nil }},
		{"CaptureCleanup", func(deps *APIDependencies) { deps.CaptureCleanup = nil }},
		{"PatientAccessHandler", func(deps *APIDependencies) { deps.PatientAccessHandler = nil }},
		{"PatientHandler", func(deps *APIDependencies) { deps.PatientHandler = nil }},
		{"PatientProblemHandler", func(deps *APIDependencies) { deps.PatientProblemHandler = nil }},
		{"LaboratoryHandler", func(deps *APIDependencies) { deps.LaboratoryHandler = nil }},
		{"ExamsHandler", func(deps *APIDependencies) { deps.ExamsHandler = nil }},
		{"StandaloneLabExtractionHandler", func(deps *APIDependencies) { deps.StandaloneLabExtractionHandler = nil }},
		{"TemporaryExtractionHandler", func(deps *APIDependencies) { deps.TemporaryExtractionHandler = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := completeTestDependencies()
			test.clear(deps)
			router := gin.New()
			_, err := SetupRoutes(router, deps)
			if err == nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("erro = %v; esperava dependência %s", err, test.name)
			}
			if routes := router.Routes(); len(routes) != 0 {
				t.Fatalf("rotas registradas após falha de validação: %+v", routes)
			}
		})
	}
}

func TestValidateAPIDependenciesReportsNilSetDeterministically(t *testing.T) {
	const want = "dependências obrigatórias da API ausentes: Auth, Account, AccountHandler, CaptureHandler, CaptureAuth, CaptureCleanup, PatientAccessHandler, PatientHandler, PatientProblemHandler, LaboratoryHandler, ExamsHandler, StandaloneLabExtractionHandler, StandaloneImagingExtractionHandler, TemporaryExtractionHandler"
	err := validateAPIDependencies(nil)
	if err == nil || err.Error() != want {
		t.Fatalf("erro = %q; esperado %q", err, want)
	}
}

func TestNewReturnsBootstrapErrorForMissingDependencies(t *testing.T) {
	app, err := New(Options{})
	if err == nil || app != nil {
		t.Fatalf("app = %v, erro = %v; esperava falha de bootstrap", app, err)
	}
	if !strings.Contains(err.Error(), "Auth") || !strings.Contains(err.Error(), "StandaloneLabExtractionHandler") {
		t.Fatalf("erro não identifica dependências ausentes: %v", err)
	}
}
