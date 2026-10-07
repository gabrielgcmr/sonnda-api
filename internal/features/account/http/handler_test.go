// internal/features/account/http/handler_test.go
package accounthttp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	authdomain "github.com/gabrielgcmr/sonnda/internal/features/auth/domain"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

func TestAccountPresentationPreservesNullProfileAndUsesRequestIdentity(t *testing.T) {
	a, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	email := "provider@example.test"
	ctx := authhttp.ContextWithIdentity(t.Context(), &authdomain.Identity{Issuer: "issuer", Subject: "subject", Email: &email})
	response := accountResponseFromDomain(ctx, a)
	if response.AuthIssuer != "issuer" || response.AuthSubject != "subject" || response.Email == nil || *response.Email != email {
		t.Fatal("request identity not preserved")
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"full_name", "birth_date", "cpf", "phone"} {
		if value, exists := body[key]; !exists || value != nil {
			t.Fatalf("%s should be null, got %v", key, value)
		}
	}
}

type lookupResolver struct {
	value *accountdomain.Account
	err   error
}

func (r lookupResolver) ResolveOrProvision(context.Context, account.AccountResolveInput) (*accountdomain.Account, error) {
	return r.value, r.err
}

func TestRegisteredAccountResolverRejectsDeactivatedAccount(t *testing.T) {
	m := NewMiddleware(lookupResolver{err: accountErrorForbidden()})
	if resolved, err := m.ResolveRegisteredAccount(t.Context(), &authdomain.Identity{Issuer: "issuer", Subject: "subject"}); resolved != nil || err == nil {
		t.Fatalf("deactivated account resolved: %+v %v", resolved, err)
	}
}

func accountErrorForbidden() error {
	return apperr.Forbidden("conta desativada")
}
