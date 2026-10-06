// internal/features/patient/http/create_handler_test.go
package patienthttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	helpers "github.com/gabrielgcmr/sonnda/internal/api/helpers"
	patientcreation "github.com/gabrielgcmr/sonnda/internal/application/usecase/patientcreation"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type creationUseCaseStub struct {
	accountID uuid.UUID
	input     patientcreation.Input
	called    bool
}

func (s *creationUseCaseStub) Execute(
	_ context.Context,
	accountID uuid.UUID,
	input patientcreation.Input,
) (*profiledomain.Patient, error) {
	s.called = true
	s.accountID = accountID
	s.input = input
	return &profiledomain.Patient{ID: uuid.New()}, nil
}

func TestCreateRequiresExplicitRelationshipType(t *testing.T) {
	creator := &creationUseCaseStub{}
	response := performCreationRequest(t, creator, `{
		"cpf":"12345678901",
		"full_name":"Joana Silva",
		"birth_date":"1990-01-01",
		"gender":"FEMALE",
		"race":"WHITE"
	}`)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
	}
	if creator.called {
		t.Fatal("use case must not run without relation_type")
	}
}

func TestCreateForwardsRelationshipTypeToAccessInput(t *testing.T) {
	creator := &creationUseCaseStub{}
	response := performCreationRequest(t, creator, `{
		"cpf":"12345678901",
		"full_name":"Joana Silva",
		"birth_date":"1990-01-01",
		"gender":"FEMALE",
		"race":"WHITE",
		"relation_type":"family"
	}`)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if !creator.called || creator.input.Access.RelationType != "family" {
		t.Fatalf("unexpected access input: %+v", creator.input.Access)
	}
}

func performCreationRequest(
	t *testing.T,
	creator patientcreation.UseCase,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	account := &accountdomain.Account{ID: uuid.New()}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), account))
		c.Next()
	})
	api := humagin.New(router, huma.DefaultConfig("test", "test"))
	NewCreationHandler(creator).RegisterHumaRoutes(api, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/patients", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}
