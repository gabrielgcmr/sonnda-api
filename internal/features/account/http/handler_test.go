// internal/features/account/http/handler_test.go
package accounthttp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	authdomain "github.com/gabrielgcmr/sonnda/internal/features/auth/domain"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

func TestAccountPresentationUsesNestedProfileAndRequestEmail(t *testing.T) {
	a, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	email := "provider@example.test"
	ctx := authhttp.ContextWithIdentity(t.Context(), &authdomain.Identity{Issuer: "issuer", Subject: "subject", Email: &email})
	response := accountResponseFromDomain(ctx, a)
	if response.Email == nil || *response.Email != email || response.OnboardingCompleted {
		t.Fatal("request identity or onboarding state not preserved")
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if _, exists := body["auth_issuer"]; exists {
		t.Fatal("authentication issuer leaked into account response")
	}
	if _, exists := body["full_name"]; exists {
		t.Fatal("profile field remained at response root")
	}
	profile, ok := body["profile"].(map[string]any)
	if !ok {
		t.Fatalf("missing nested profile: %s", data)
	}
	for _, key := range []string{"full_name", "birth_date", "cpf", "phone"} {
		if value, exists := profile[key]; !exists || value != nil {
			t.Fatalf("%s should be null, got %v", key, value)
		}
	}
}

func TestPatchRequestDistinguishesOmittedNullAndValue(t *testing.T) {
	var request updateAccountRequest
	if err := json.Unmarshal([]byte(`{"full_name":null,"cpf":"","phone":"11999999999"}`), &request); err != nil {
		t.Fatal(err)
	}
	if !request.FullName.Set || request.FullName.Value != nil || request.BirthDate.Set ||
		!request.CPF.Set || request.CPF.Value == nil || *request.CPF.Value != "" ||
		!request.Phone.Set || request.Phone.Value == nil || *request.Phone.Value != "11999999999" {
		t.Fatalf("unexpected request state: %+v", request)
	}
}

type handlerAccountService struct {
	updated           *accountdomain.Account
	updateInput       account.AccountUpdateInput
	deactivatedIssuer string
	deactivatedSub    string
}

func (s *handlerAccountService) Update(_ context.Context, input account.AccountUpdateInput) (*accountdomain.Account, error) {
	s.updateInput = input
	return s.updated, nil
}

func (s *handlerAccountService) DeactivateByIdentity(_ context.Context, issuer, subject string) error {
	s.deactivatedIssuer = issuer
	s.deactivatedSub = subject
	return nil
}

func TestPatchMapsNullableFieldsAndSetsNoStore(t *testing.T) {
	current, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	service := &handlerAccountService{updated: current}
	handler := NewHandler(service, nil)
	ctx := helpers.ContextWithCurrentAccount(t.Context(), current)
	ctx = authhttp.ContextWithIdentity(ctx, &authdomain.Identity{Issuer: "issuer", Subject: "subject"})
	var request updateAccountRequest
	if err := json.Unmarshal([]byte(`{"full_name":"Ana Silva","birth_date":"1990-01-02","cpf":null}`), &request); err != nil {
		t.Fatal(err)
	}
	output, err := handler.updateCurrentAccount(ctx, &updateAccountInput{Body: request})
	if err != nil {
		t.Fatal(err)
	}
	input := service.updateInput
	if !input.FullName.Set || input.FullName.Value == nil || *input.FullName.Value != "Ana Silva" ||
		!input.BirthDate.Set || input.BirthDate.Value == nil || input.BirthDate.Value.Format(time.DateOnly) != "1990-01-02" ||
		!input.CPF.Set || input.CPF.Value != nil || input.Phone.Set {
		t.Fatalf("unexpected update input: %+v", input)
	}
	if output.CacheControl != accountCacheControl {
		t.Fatalf("cache control = %q", output.CacheControl)
	}
}

func TestDeleteUsesIdentityWithoutAccountResolution(t *testing.T) {
	service := &handlerAccountService{}
	handler := NewHandler(service, nil)
	ctx := authhttp.ContextWithIdentity(t.Context(), &authdomain.Identity{Issuer: "issuer", Subject: "subject"})
	if _, err := handler.deleteCurrentAccount(ctx, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if service.deactivatedIssuer != "issuer" || service.deactivatedSub != "subject" {
		t.Fatalf("deactivated %q/%q", service.deactivatedIssuer, service.deactivatedSub)
	}
}

type lookupResolver struct {
	value *accountdomain.Account
	err   error
}

func (r lookupResolver) ResolveOrProvision(context.Context, account.AccountResolveInput) (*accountdomain.Account, error) {
	return r.value, r.err
}

func TestAccountResolverPreservesDeactivatedError(t *testing.T) {
	middleware := NewMiddleware(lookupResolver{err: apperr.AccountDeactivated()})
	if resolved, err := middleware.ResolveAccount(t.Context(), &authdomain.Identity{Issuer: "issuer", Subject: "subject"}); resolved != nil || apperr.ErrorCodeOf(err) != apperr.ACCOUNT_DEACTIVATED {
		t.Fatalf("deactivated account resolved: %+v %v", resolved, err)
	}
}
