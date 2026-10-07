// internal/features/account/postgres/repository_integration_test.go
package accountpostgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResolveOrProvisionConcurrencyIntegration(t *testing.T) {
	repository := newAccountTestRepository(t)
	service := account.New(repository)
	const workers = 16
	ids := make(chan uuid.UUID, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup

	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			resolved, err := service.ResolveOrProvision(t.Context(), account.AccountResolveInput{
				Issuer: "issuer", Subject: "concurrent-subject",
			})
			if err != nil {
				errs <- err
				return
			}
			ids <- resolved.ID
		}()
	}
	group.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	var expected uuid.UUID
	for id := range ids {
		if expected == uuid.Nil {
			expected = id
		}
		if id != expected {
			t.Fatalf("concurrent resolution returned %s and %s", expected, id)
		}
	}
	var accountCount int
	if err := repository.db.QueryRow(t.Context(), "SELECT count(*) FROM accounts").Scan(&accountCount); err != nil {
		t.Fatal(err)
	}
	if accountCount != 1 {
		t.Fatalf("provisioned %d accounts", accountCount)
	}
}

func TestResolveOrProvisionEmailAndDeactivationIntegration(t *testing.T) {
	repository := newAccountTestRepository(t)
	service := account.New(repository)
	firstEmail := "first@example.test"
	created, err := service.ResolveOrProvision(t.Context(), account.AccountResolveInput{
		Issuer: "issuer", Subject: "subject", Email: &firstEmail,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondEmail := "second@example.test"
	resolved, err := service.ResolveOrProvision(t.Context(), account.AccountResolveInput{
		Issuer: "issuer", Subject: "subject", Email: &secondEmail,
	})
	if err != nil || resolved.ID != created.ID {
		t.Fatalf("resolved=%+v error=%v", resolved, err)
	}
	identity, err := repository.FindIdentity(t.Context(), "issuer", "subject")
	if err != nil || identity.Email == nil || *identity.Email != secondEmail {
		t.Fatalf("identity=%+v error=%v", identity, err)
	}
	if err := repository.SoftDelete(t.Context(), created.ID); err != nil {
		t.Fatal(err)
	}
	resolved, err = service.ResolveOrProvision(t.Context(), account.AccountResolveInput{Issuer: "issuer", Subject: "subject"})
	var appErr *apperr.AppError
	if resolved != nil || !errors.As(err, &appErr) || appErr.Kind != apperr.ACCESS_DENIED {
		t.Fatalf("resolved=%+v error=%v", resolved, err)
	}
}

func TestAccountIdentityPersistenceIntegration(t *testing.T) {
	r := newAccountTestRepository(t)
	ctx := t.Context()
	a, err := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	if err != nil {
		t.Fatal(err)
	}
	i, err := accountdomain.NewIdentity(a.ID, "issuer", "subject", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, a, i); err != nil {
		t.Fatal(err)
	}
	found, err := r.FindByAuthIdentity(ctx, i.Issuer, i.Subject)
	if err != nil || found == nil || found.ID != a.ID || found.Profile != (accountdomain.Profile{}) {
		t.Fatalf("minimal account round trip: %+v %v", found, err)
	}
	identity, err := r.FindIdentity(ctx, i.Issuer, i.Subject)
	if err != nil || identity == nil || identity.AccountID != a.ID || identity.Email != nil {
		t.Fatalf("identity round trip: %+v %v", identity, err)
	}

	duplicate, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	duplicateIdentity, _ := accountdomain.NewIdentity(duplicate.ID, i.Issuer, i.Subject, nil)
	if err := r.Create(ctx, duplicate, duplicateIdentity); !errors.Is(err, account.ErrAccountAlreadyExists) {
		t.Fatalf("duplicate identity: %v", err)
	}
	if orphan, err := r.FindByID(ctx, duplicate.ID); err != nil || orphan != nil {
		t.Fatalf("failed creation left an orphan: %+v %v", orphan, err)
	}
	duplicateIdentity.Issuer = "another-issuer"
	if err := r.Create(ctx, duplicate, duplicateIdentity); err != nil {
		t.Fatal(err)
	}
	if other, err := r.FindByAuthIdentity(ctx, duplicateIdentity.Issuer, i.Subject); err != nil || other == nil || other.ID != duplicate.ID {
		t.Fatalf("issuer separation: %+v %v", other, err)
	}

	name := "Ana Silva"
	birth := time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)
	if _, err := a.ApplyProfile(accountdomain.Profile{FullName: &name, BirthDate: &birth}); err != nil {
		t.Fatal(err)
	}
	if err := r.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	if !a.OnboardingCompleted() || a.Profile.CPF != nil || a.Profile.Phone != nil {
		t.Fatal("optional values or completion changed after update")
	}
	if _, err := a.ApplyProfile(accountdomain.Profile{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	if a.Profile != (accountdomain.Profile{}) || a.OnboardingCompleted() {
		t.Fatal("NULL clearing was not preserved")
	}
	if err := r.SoftDelete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	found, err = r.FindByAuthIdentity(ctx, i.Issuer, i.Subject)
	if err != nil || found == nil || found.ID != a.ID || found.DeletedAt == nil {
		t.Fatalf("deactivation lost identity linkage: %+v %v", found, err)
	}
	identityAfter, err := r.FindIdentity(ctx, i.Issuer, i.Subject)
	if err != nil || identityAfter == nil || identityAfter.AccountID != identity.AccountID || !identityAfter.UpdatedAt.Equal(identity.UpdatedAt) {
		t.Fatalf("profile writes modified identity: %+v %v", identityAfter, err)
	}
}

func newAccountTestRepository(t *testing.T) *Repository {
	t.Helper()
	rawURL := os.Getenv("ACCOUNTS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set ACCOUNTS_TEST_DATABASE_URL to an isolated local Postgres")
	}
	databaseURL, err := url.Parse(rawURL)
	if err != nil || (databaseURL.Hostname() != "127.0.0.1" && databaseURL.Hostname() != "localhost") {
		t.Fatal("integration tests require a local Postgres URL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	schema := "account_test_" + uuid.NewString()[:8]
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	params := databaseURL.Query()
	params.Set("search_path", schema)
	databaseURL.RawQuery = params.Encode()
	pool, err := pgxpool.New(ctx, databaseURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	schemaPath := filepath.Join("..", "..", "..", "infrastructure", "database", "postgres", "sqlc", "sql", "schema", "accounts.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schemaSQL)); err != nil {
		t.Fatal(err)
	}
	return New(pool)
}
