// internal/features/capture/postgres/repository_integration_test.go
//go:build integration

package capturepostgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestCaptureMigrationAndRepositoryLifecycle(t *testing.T) {
	client, repo, accountID, otherAccountID, schema := captureTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	pairingHash := hashWithByte(1)
	session, err := capturedomain.NewSession(accountID, pairingHash, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	duplicate, err := capturedomain.NewSession(accountID, hashWithByte(2), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateSession(ctx, duplicate); !errors.Is(err, capture.ErrStateConflict) {
		t.Fatalf("second active session error = %v", err)
	}

	claimed, err := session.Claim(hashWithByte(3), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := capturedomain.NewSessionClaim(hashWithByte(3), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	storedSession, err := repo.ClaimSession(ctx, pairingHash, claim, now.Add(-time.Minute))
	if err != nil || storedSession.ClaimedAt == nil {
		t.Fatalf("claim session: %+v %v", storedSession, err)
	}
	if _, err = repo.AuthenticateUpload(ctx, session.ID, claimed.UploadTokenHash, now.Add(2*time.Minute), now); err != nil {
		t.Fatal(err)
	}

	item, err := capturedomain.NewCapture(capturedomain.NewCaptureParams{
		AccountID: accountID, CaptureSessionID: session.ID, OriginalFilename: "exam.pdf",
		MIMEType: "application/pdf", SizeBytes: 512, CreatedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateCapture(ctx, item); err != nil {
		t.Fatal(err)
	}
	available, err := repo.SetCaptureAvailable(ctx, item.ID, "gs://private/captures/file", now.Add(3*time.Minute))
	if err != nil || available.Status != capturedomain.StatusAvailable {
		t.Fatalf("available capture: %+v %v", available, err)
	}
	items, err := repo.ListCaptures(ctx, accountID, now.Add(4*time.Minute), capture.Pagination{Limit: 20})
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("listed captures: %+v %v", items, err)
	}

	wrongOwner, err := capturedomain.NewCapture(capturedomain.NewCaptureParams{
		AccountID: otherAccountID, CaptureSessionID: session.ID, OriginalFilename: "wrong.png",
		MIMEType: "image/png", SizeBytes: 20, CreatedAt: now.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateCapture(ctx, wrongOwner); err == nil {
		t.Fatal("capture accepted with a session owned by another account")
	}

	replacement, err := capturedomain.NewSession(accountID, hashWithByte(4), now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.WithinTransaction(ctx, func(txRepo capture.Repository) error {
		if _, err := txRepo.RevokeSessionsByAccount(ctx, accountID, now.Add(5*time.Minute)); err != nil {
			return err
		}
		return txRepo.CreateSession(ctx, replacement)
	}); err != nil {
		t.Fatal(err)
	}
	current, err := repo.FindCurrentSession(ctx, accountID)
	if err != nil || current.ID != replacement.ID {
		t.Fatalf("current session: %+v %v", current, err)
	}

	for _, table := range []string{"capture_sessions", "captures"} {
		var rls, authenticatedAllowed, anonymousAllowed bool
		qualified := pgx.Identifier{schema, table}.Sanitize()
		err = client.Pool().QueryRow(ctx, `SELECT relrowsecurity,
            has_table_privilege('authenticated', oid, 'SELECT'),
            has_table_privilege('anon', oid, 'SELECT')
            FROM pg_class WHERE oid=$1::regclass`, qualified).Scan(&rls, &authenticatedAllowed, &anonymousAllowed)
		if err != nil || !rls || authenticatedAllowed || anonymousAllowed {
			t.Fatalf("table exposure %s: RLS=%v authenticated=%v anon=%v err=%v", table, rls, authenticatedAllowed, anonymousAllowed, err)
		}
	}
}

func captureTestDatabase(t *testing.T) (*postgress.Client, *Repository, uuid.UUID, uuid.UUID, string) {
	t.Helper()
	rawURL := os.Getenv("CAPTURES_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set CAPTURES_TEST_DATABASE_URL to an isolated local Supabase Postgres")
	}
	databaseURL, err := url.Parse(rawURL)
	if err != nil || (databaseURL.Hostname() != "127.0.0.1" && databaseURL.Hostname() != "localhost") {
		t.Fatal("capture integration tests require a local Supabase Postgres URL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	schema := "capture_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, dropErr := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); dropErr != nil {
			t.Error(dropErr)
		}
	})
	if _, err = admin.Exec(ctx, `DO $$ BEGIN
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='anon') THEN CREATE ROLE anon; END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN CREATE ROLE authenticated; END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='service_role') THEN CREATE ROLE service_role; END IF;
        END $$;`); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, "CREATE TABLE "+identifier+".accounts (id uuid PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	migrationPath := filepath.Join("..", "..", "..", "..", "supabase", "migrations", "20261009132454_create_capture_sessions_and_captures.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	migrationSQL := strings.ReplaceAll(string(migration), "public.", identifier+".")
	if _, err = admin.Exec(ctx, migrationSQL); err != nil {
		t.Fatal(err)
	}

	params := databaseURL.Query()
	params.Set("search_path", schema)
	databaseURL.RawQuery = params.Encode()
	client, err := postgress.NewClient(postgress.Config{DatabaseURL: databaseURL.String(), MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	accountID, otherAccountID := uuid.New(), uuid.New()
	if _, err = client.Pool().Exec(ctx, "INSERT INTO accounts(id) VALUES ($1), ($2)", accountID, otherAccountID); err != nil {
		t.Fatal(err)
	}
	return client, NewRepository(client), accountID, otherAccountID, schema
}

func hashWithByte(value byte) []byte {
	hash := make([]byte, capturedomain.CredentialHashBytes)
	for i := range hash {
		hash[i] = value
	}
	return hash
}
