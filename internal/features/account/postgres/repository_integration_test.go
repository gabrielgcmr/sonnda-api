// internal/features/account/postgres/repository_integration_test.go
package accountpostgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	if resolved != nil || !errors.As(err, &appErr) || appErr.Kind != apperr.ACCOUNT_DEACTIVATED {
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

func TestAccountIdentityOnboardingMigrationIntegration(t *testing.T) {
	rawURL, _ := accountTestDatabaseURL(t)
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })

	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	schema := "account_migration_" + suffix
	privateSchema := "account_private_" + suffix
	schemaIdentifier := pgx.Identifier{schema}.Sanitize()
	privateIdentifier := pgx.Identifier{privateSchema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaIdentifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+privateIdentifier+" CASCADE"); err != nil {
			t.Error(err)
		}
		if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schemaIdentifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	})

	legacySchema := strings.ReplaceAll(`
CREATE TABLE public.users (
    id uuid PRIMARY KEY,
    auth_issuer text NOT NULL,
    auth_subject text NOT NULL,
    email text NOT NULL CONSTRAINT users_email_key UNIQUE,
    account_type text NOT NULL DEFAULT 'basic_care',
    full_name text NOT NULL,
    birth_date date NOT NULL,
    cpf text NOT NULL UNIQUE,
    phone text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX idx_users_email ON public.users(email);
CREATE UNIQUE INDEX idx_users_auth_identity ON public.users(auth_issuer, auth_subject);
ALTER TABLE public.users ENABLE ROW LEVEL SECURITY;
CREATE POLICY "Users can CRUD own profile" ON public.users FOR ALL USING (true) WITH CHECK (true);

CREATE TABLE public.patients (
    id uuid PRIMARY KEY,
    owner_user_id uuid REFERENCES public.users(id),
    created_by_user_id uuid NOT NULL REFERENCES public.users(id)
);
CREATE TABLE public.patient_access (
    patient_id uuid NOT NULL REFERENCES public.patients(id),
    grantee_id uuid NOT NULL REFERENCES public.users(id),
    revoked_at timestamptz
);
ALTER TABLE public.patients ENABLE ROW LEVEL SECURITY;
CREATE POLICY "users can create patients" ON public.patients FOR INSERT WITH CHECK (true);
CREATE POLICY "users can view linked patients" ON public.patients FOR SELECT USING (true);

CREATE FUNCTION public.current_app_user_id()
RETURNS uuid LANGUAGE sql STABLE AS $$ SELECT NULL::uuid $$;
`, "public.", schema+".")
	if _, err := admin.Exec(ctx, legacySchema); err != nil {
		t.Fatal(err)
	}

	activeID := uuid.New()
	deactivatedID := uuid.New()
	patientID := uuid.New()
	issuer := "https://project.supabase.co/auth/v1"
	if _, err := admin.Exec(ctx, `
INSERT INTO `+schemaIdentifier+`.users
    (id, auth_issuer, auth_subject, email, full_name, birth_date, cpf, phone, deleted_at)
VALUES
    ($1, $3, 'active-subject', 'active@example.test', 'Active Account', DATE '1990-01-02', '12345678901', '11999999999', NULL),
    ($2, $3, 'deleted-subject', 'deleted@example.test', 'Deleted Account', DATE '1980-03-04', '10987654321', '11888888888', now());
INSERT INTO `+schemaIdentifier+`.patients (id, owner_user_id, created_by_user_id) VALUES ($4, $1, $1);
INSERT INTO `+schemaIdentifier+`.patient_access (patient_id, grantee_id) VALUES ($4, $2);
`, activeID, deactivatedID, issuer, patientID); err != nil {
		t.Fatal(err)
	}

	migrationPath := filepath.Join("..", "..", "..", "..", "supabase", "migrations", "20261006110649_account_identity_onboarding.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	migrationSQL := strings.ReplaceAll(string(migration), "public.", schema+".")
	migrationSQL = strings.ReplaceAll(migrationSQL, "private", privateSchema)
	if _, err := admin.Exec(ctx, migrationSQL); err != nil {
		t.Fatal(err)
	}

	var accountCount, identityCount, preservedForeignKeys int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM "+schemaIdentifier+".accounts").Scan(&accountCount); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM "+schemaIdentifier+".account_identities").Scan(&identityCount); err != nil {
		t.Fatal(err)
	}
	if accountCount != 2 || identityCount != 2 {
		t.Fatalf("migration lost rows: accounts=%d identities=%d", accountCount, identityCount)
	}
	if err := admin.QueryRow(ctx, `
SELECT count(*)
FROM pg_constraint c
JOIN pg_class source ON source.oid = c.conrelid
JOIN pg_namespace source_ns ON source_ns.oid = source.relnamespace
JOIN pg_class target ON target.oid = c.confrelid
WHERE c.contype = 'f' AND source_ns.nspname = $1 AND target.relname = 'accounts'
`, schema).Scan(&preservedForeignKeys); err != nil {
		t.Fatal(err)
	}
	if preservedForeignKeys != 3 {
		t.Fatalf("expected 3 preserved account foreign keys, got %d", preservedForeignKeys)
	}

	var legacyColumnCount, nullableProfileColumns int
	if err := admin.QueryRow(ctx, `
SELECT count(*) FILTER (WHERE column_name IN ('auth_issuer', 'auth_subject', 'email')),
       count(*) FILTER (WHERE column_name IN ('full_name', 'birth_date', 'cpf', 'phone') AND is_nullable = 'YES')
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = 'accounts'
`, schema).Scan(&legacyColumnCount, &nullableProfileColumns); err != nil {
		t.Fatal(err)
	}
	if legacyColumnCount != 0 || nullableProfileColumns != 4 {
		t.Fatalf("unexpected migrated columns: legacy=%d nullable_profile=%d", legacyColumnCount, nullableProfileColumns)
	}

	var anonCanRead, authenticatedCanWrite bool
	if err := admin.QueryRow(ctx, `
SELECT has_table_privilege('anon', $1, 'SELECT'),
       has_table_privilege('authenticated', $2, 'INSERT')
`, schema+".accounts", schema+".account_identities").Scan(&anonCanRead, &authenticatedCanWrite); err != nil {
		t.Fatal(err)
	}
	if anonCanRead || authenticatedCanWrite {
		t.Fatalf("account tables remain exposed: anon_read=%t authenticated_write=%t", anonCanRead, authenticatedCanWrite)
	}

	claims := `{"iss":"` + issuer + `","sub":"active-subject"}`
	if _, err := admin.Exec(ctx, "SELECT set_config('request.jwt.claims', $1, false)", claims); err != nil {
		t.Fatal(err)
	}
	var resolvedID pgtype.UUID
	if err := admin.QueryRow(ctx, "SELECT "+privateIdentifier+".current_app_user_id()").Scan(&resolvedID); err != nil {
		t.Fatal(err)
	}
	if !resolvedID.Valid || resolvedID.Bytes != activeID {
		t.Fatalf("active identity resolved to %+v", resolvedID)
	}
	claims = `{"iss":"` + issuer + `","sub":"deleted-subject"}`
	if _, err := admin.Exec(ctx, "SELECT set_config('request.jwt.claims', $1, false)", claims); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, "SELECT "+privateIdentifier+".current_app_user_id()").Scan(&resolvedID); err != nil {
		t.Fatal(err)
	}
	if resolvedID.Valid {
		t.Fatalf("deactivated identity resolved to %v", resolvedID.Bytes)
	}
}

func newAccountTestRepository(t *testing.T) *Repository {
	t.Helper()
	rawURL, databaseURL := accountTestDatabaseURL(t)
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

func accountTestDatabaseURL(t *testing.T) (string, *url.URL) {
	t.Helper()
	rawURL := os.Getenv("ACCOUNTS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set ACCOUNTS_TEST_DATABASE_URL to an isolated local Supabase Postgres")
	}
	databaseURL, err := url.Parse(rawURL)
	if err != nil || (databaseURL.Hostname() != "127.0.0.1" && databaseURL.Hostname() != "localhost") {
		t.Fatal("account integration tests require a local Supabase Postgres URL")
	}
	return rawURL, databaseURL
}
