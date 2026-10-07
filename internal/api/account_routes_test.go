// internal/api/account_routes_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	authdomain "github.com/gabrielgcmr/sonnda/internal/features/auth/domain"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	"github.com/gin-gonic/gin"
)

type routeAccountService struct {
	account         *accountdomain.Account
	resolveCalls    int
	updateCalls     int
	deactivateCalls int
	updateInput     account.AccountUpdateInput
}

func (s *routeAccountService) ResolveOrProvision(context.Context, account.AccountResolveInput) (*accountdomain.Account, error) {
	s.resolveCalls++
	return s.account, nil
}

func (s *routeAccountService) Update(_ context.Context, input account.AccountUpdateInput) (*accountdomain.Account, error) {
	s.updateCalls++
	s.updateInput = input
	return s.account, nil
}

func (s *routeAccountService) DeactivateByIdentity(context.Context, string, string) error {
	s.deactivateCalls++
	return nil
}

func TestAccountHTTPContractAndOnboardingBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	current, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	service := &routeAccountService{account: current}
	router := gin.New()
	SetupRoutes(router, &APIDependencies{
		Auth:           authenticatedTestMiddleware(),
		Account:        accounthttp.NewMiddleware(service),
		AccountHandler: accounthttp.NewHandler(service, nil),
	})

	response := accountRequest(router, http.MethodGet, "/me", "")
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("GET /me: %d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["onboarding_completed"] != false || body["profile"] == nil || body["email"] != "person@example.test" {
		t.Fatalf("unexpected GET /me body: %s", response.Body.String())
	}

	response = accountRequest(router, http.MethodPatch, "/me", `{"full_name":null,"cpf":""}`)
	if response.Code != http.StatusOK || service.updateCalls != 1 {
		t.Fatalf("PATCH /me: %d body=%s updates=%d", response.Code, response.Body.String(), service.updateCalls)
	}
	if !service.updateInput.FullName.Set || service.updateInput.FullName.Value != nil ||
		!service.updateInput.CPF.Set || service.updateInput.CPF.Value == nil || *service.updateInput.CPF.Value != "" ||
		service.updateInput.BirthDate.Set || service.updateInput.Phone.Set {
		t.Fatalf("PATCH did not preserve field state: %+v", service.updateInput)
	}

	response = accountRequest(router, http.MethodPatch, "/me", `{"email":"forbidden@example.test"}`)
	if response.Code != http.StatusUnprocessableEntity || service.updateCalls != 1 {
		t.Fatalf("immutable field accepted: %d body=%s updates=%d", response.Code, response.Body.String(), service.updateCalls)
	}
	for _, invalidBody := range []string{`{"full_name":""}`, `{"birth_date":""}`} {
		response = accountRequest(router, http.MethodPatch, "/me", invalidBody)
		if response.Code != http.StatusUnprocessableEntity || service.updateCalls != 1 {
			t.Fatalf("invalid empty profile field accepted: body=%s status=%d response=%s updates=%d", invalidBody, response.Code, response.Body.String(), service.updateCalls)
		}
	}

	resolveCalls := service.resolveCalls
	response = accountRequest(router, http.MethodDelete, "/me", "")
	if response.Code != http.StatusNoContent || service.deactivateCalls != 1 || service.resolveCalls != resolveCalls {
		t.Fatalf("DELETE /me: %d resolves=%d/%d deactivations=%d body=%s", response.Code, service.resolveCalls, resolveCalls, service.deactivateCalls, response.Body.String())
	}

	response = accountRequest(router, http.MethodGet, "/patients", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("pending onboarding reached business route: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "ONBOARDING_REQUIRED") {
		t.Fatalf("internal error code leaked: %s", response.Body.String())
	}
}

func authenticatedTestMiddleware() *authhttp.Middleware {
	return authhttp.NewMiddleware(func(context.Context, string) (*authdomain.Identity, error) {
		email := "person@example.test"
		return &authdomain.Identity{Issuer: "issuer", Subject: "subject", Email: &email}, nil
	})
}

func accountRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer token")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
