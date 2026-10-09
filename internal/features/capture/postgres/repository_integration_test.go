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
	if _, err = repo.AuthenticateMobile(ctx, claimed.UploadTokenHash, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("authenticate mobile: %v", err)
	}
	if _, err = repo.AuthenticateUpload(ctx, claimed.UploadTokenHash, now.Add(2*time.Minute), now); err != nil {
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
	available, err := repo.SetCaptureAvailable(ctx, item.ID, "supabase://captures/account/file.pdf", now.Add(3*time.Minute))
	if err != nil || available.Status != capturedomain.StatusAvailable {
		t.Fatalf("available capture: %+v %v", available, err)
	}
	items, err := repo.ListCaptures(ctx, accountID, now.Add(4*time.Minute), capture.Pagination{Limit: 20})
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("listed captures: %+v %v", items, err)
	}
	if _, err = repo.FindAvailableCapture(ctx, otherAccountID, item.ID, now.Add(4*time.Minute)); !errors.Is(err, capture.ErrCaptureNotFound) {
		t.Fatalf("other account obtained capture: %v", err)
	}
	if found, findErr := repo.FindAvailableCapture(ctx, accountID, item.ID, now.Add(4*time.Minute)); findErr != nil || found.ID != item.ID {
		t.Fatalf("owner did not obtain capture: %+v %v", found, findErr)
	}

	expiredItem, err := capturedomain.NewCapture(capturedomain.NewCaptureParams{
		AccountID: accountID, CaptureSessionID: session.ID, OriginalFilename: "expired.pdf",
		MIMEType: "application/pdf", SizeBytes: 128, CreatedAt: now.Add(-25 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateCapture(ctx, expiredItem); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.SetCaptureAvailable(ctx, expiredItem.ID, "supabase://captures/account/expired.pdf", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err = repo.ListCaptures(ctx, accountID, now.Add(4*time.Minute), capture.Pagination{Limit: 20})
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("expired capture was listed: %+v %v", items, err)
	}
	if _, err = repo.FindAvailableCapture(ctx, accountID, expiredItem.ID, now.Add(4*time.Minute)); !errors.Is(err, capture.ErrCaptureNotFound) {
		t.Fatalf("expired capture received file access: %v", err)
	}

	if _, err = repo.MarkOwnedCaptureDeleting(ctx, otherAccountID, item.ID, now.Add(5*time.Minute)); !errors.Is(err, capture.ErrCaptureNotFound) {
		t.Fatalf("other account marked capture for deletion: %v", err)
	}
	deleting, err := repo.MarkOwnedCaptureDeleting(ctx, accountID, item.ID, now.Add(5*time.Minute))
	if err != nil || deleting.Status != capturedomain.StatusDeleting {
		t.Fatalf("owner could not mark capture for deletion: %+v %v", deleting, err)
	}
	if _, err = repo.MarkOwnedCaptureDeleting(ctx, accountID, item.ID, now.Add(6*time.Minute)); err != nil {
		t.Fatalf("repeated deletion mark should be resumable: %v", err)
	}
	if err = repo.DeleteOwnedCapture(ctx, otherAccountID, item.ID); !errors.Is(err, capture.ErrCaptureNotFound) {
		t.Fatalf("other account deleted capture row: %v", err)
	}
	if err = repo.DeleteOwnedCapture(ctx, accountID, item.ID); err != nil {
		t.Fatalf("owner could not delete capture row: %v", err)
	}
	if err = repo.DeleteOwnedCapture(ctx, accountID, item.ID); !errors.Is(err, capture.ErrCaptureNotFound) {
		t.Fatalf("repeated database deletion returned unexpected error: %v", err)
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
	if err = repo.TouchDesktop(ctx, replacement.ID, otherAccountID, now.Add(6*time.Minute)); !errors.Is(err, capture.ErrSessionNotFound) {
		t.Fatalf("other account heartbeat error = %v", err)
	}
	if err = repo.RevokeSession(ctx, replacement.ID, otherAccountID, now.Add(6*time.Minute)); !errors.Is(err, capture.ErrSessionNotFound) {
		t.Fatalf("other account revocation error = %v", err)
	}
	current, err = repo.FindCurrentSession(ctx, accountID)
	if err != nil || current.ID != replacement.ID || current.RevokedAt != nil {
		t.Fatalf("other account changed session: %+v %v", current, err)
	}

	replacementClaim, err := capturedomain.NewSessionClaim(hashWithByte(5), now.Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ClaimSession(ctx, replacement.PairingCodeHash, replacementClaim, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.AuthenticateUpload(ctx, replacementClaim.UploadTokenHash, now.Add(7*time.Minute), now.Add(6*time.Minute)); !errors.Is(err, capture.ErrSessionNotFound) {
		t.Fatalf("upload with stale desktop presence error = %v", err)
	}
	if err = repo.TouchDesktop(ctx, replacement.ID, accountID, now.Add(7*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.AuthenticateUpload(ctx, replacementClaim.UploadTokenHash, now.Add(7*time.Minute), now.Add(6*time.Minute)); err != nil {
		t.Fatalf("upload after restored desktop presence: %v", err)
	}
	if _, err = repo.AuthenticateUpload(ctx, claimed.UploadTokenHash, now.Add(7*time.Minute), now.Add(4*time.Minute)); !errors.Is(err, capture.ErrSessionNotFound) {
		t.Fatalf("upload after session replacement error = %v", err)
	}
	reusedClaim, err := capturedomain.NewSessionClaim(hashWithByte(6), now.Add(7*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ClaimSession(ctx, replacement.PairingCodeHash, reusedClaim, now.Add(4*time.Minute)); !errors.Is(err, capture.ErrStateConflict) {
		t.Fatalf("reused pairing code error = %v", err)
	}
	if _, err = repo.ClaimSession(ctx, pairingHash, reusedClaim, now.Add(4*time.Minute)); !errors.Is(err, capture.ErrStateConflict) {
		t.Fatalf("replaced pairing code error = %v", err)
	}

	expired, err := capturedomain.NewSession(otherAccountID, hashWithByte(7), now)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateSession(ctx, expired); err != nil {
		t.Fatal(err)
	}
	expiredClaim, err := capturedomain.NewSessionClaim(hashWithByte(8), now.Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ClaimSession(ctx, expired.PairingCodeHash, expiredClaim, now.Add(4*time.Minute)); !errors.Is(err, capture.ErrStateConflict) {
		t.Fatalf("expired pairing code error = %v", err)
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

func TestClaimCaptureSessionIsSingleUseUnderConcurrency(t *testing.T) {
	_, repo, accountID, _, _ := captureTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	pairingHash := hashWithByte(20)
	session, err := capturedomain.NewSession(accountID, pairingHash, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, tokenByte := range []byte{21, 22} {
		tokenByte := tokenByte
		go func() {
			<-start
			claim, claimErr := capturedomain.NewSessionClaim(hashWithByte(tokenByte), now.Add(time.Minute))
			if claimErr == nil {
				_, claimErr = repo.ClaimSession(ctx, pairingHash, claim, now.Add(-time.Minute))
			}
			results <- claimErr
		}()
	}
	close(start)

	var successes, conflicts int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, capture.ErrStateConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent claim error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent claim results: successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestListCleanupCandidatesSelectsOnlyEligibleCaptures(t *testing.T) {
	_, repo, accountID, _, _ := captureTestDatabase(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	session, err := capturedomain.NewSession(accountID, hashWithByte(30), now.Add(-26*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	expired := createCaptureForCleanupTest(t, ctx, repo, accountID, session.ID, "expired.pdf", now.Add(-25*time.Hour))
	if _, err = repo.SetCaptureAvailable(ctx, expired.ID, *expired.StorageURI, expired.CreatedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	deleting := createCaptureForCleanupTest(t, ctx, repo, accountID, session.ID, "deleting.pdf", now.Add(-2*time.Hour))
	if _, err = repo.SetCaptureAvailable(ctx, deleting.ID, *deleting.StorageURI, deleting.CreatedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MarkCaptureDeleting(ctx, deleting.ID, deleting.CreatedAt.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	stuckUploading := createCaptureForCleanupTest(t, ctx, repo, accountID, session.ID, "stuck.pdf", now.Add(-2*time.Hour))
	recentUploading := createCaptureForCleanupTest(t, ctx, repo, accountID, session.ID, "recent.pdf", now.Add(-30*time.Minute))
	available := createCaptureForCleanupTest(t, ctx, repo, accountID, session.ID, "available.pdf", now.Add(-30*time.Minute))
	if _, err = repo.SetCaptureAvailable(ctx, available.ID, *available.StorageURI, available.CreatedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	candidates, err := repo.ListCleanupCandidates(ctx, now, now.Add(-time.Hour), 20)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uuid.UUID]struct{}{
		expired.ID:        {},
		deleting.ID:       {},
		stuckUploading.ID: {},
	}
	if len(candidates) != len(want) {
		t.Fatalf("cleanup candidates = %d, want %d: %+v", len(candidates), len(want), candidates)
	}
	for _, candidate := range candidates {
		if _, ok := want[candidate.ID]; !ok {
			t.Fatalf("capture %s unexpectedly selected for cleanup", candidate.ID)
		}
		delete(want, candidate.ID)
	}
	if len(want) != 0 {
		t.Fatalf("eligible captures not selected: %v", want)
	}
	for _, excludedID := range []uuid.UUID{recentUploading.ID, available.ID} {
		for _, candidate := range candidates {
			if candidate.ID == excludedID {
				t.Fatalf("valid capture %s selected for cleanup", excludedID)
			}
		}
	}
}

func TestDeleteExpiredSessionsRemovesOnlyUnreferencedExpiredOrRevoked(t *testing.T) {
	client, repo, _, _, _ := captureTestDatabase(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	accountIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	for _, accountID := range accountIDs {
		if _, err := client.Pool().Exec(ctx, "INSERT INTO accounts(id) VALUES ($1)", accountID); err != nil {
			t.Fatal(err)
		}
	}

	revoked := createSessionForCleanupTest(t, ctx, repo, accountIDs[0], hashWithByte(40), now.Add(-2*time.Minute))
	if err := repo.RevokeSession(ctx, revoked.ID, revoked.AccountID, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	expired := createSessionForCleanupTest(t, ctx, repo, accountIDs[1], hashWithByte(41), now.Add(-10*time.Minute))
	referenced := createSessionForCleanupTest(t, ctx, repo, accountIDs[2], hashWithByte(42), now.Add(-10*time.Minute))
	createCaptureForCleanupTest(t, ctx, repo, referenced.AccountID, referenced.ID, "referenced.pdf", now.Add(-30*time.Minute))
	valid := createSessionForCleanupTest(t, ctx, repo, accountIDs[3], hashWithByte(43), now)

	deleted, err := repo.DeleteExpiredSessions(ctx, now, 20)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("deleted sessions = %d, want 2", deleted)
	}

	for _, sessionID := range []uuid.UUID{revoked.ID, expired.ID} {
		if _, findErr := repo.FindSession(ctx, sessionID); !errors.Is(findErr, capture.ErrSessionNotFound) {
			t.Fatalf("session %s should have been deleted, got %v", sessionID, findErr)
		}
	}
	for _, sessionID := range []uuid.UUID{referenced.ID, valid.ID} {
		if _, findErr := repo.FindSession(ctx, sessionID); findErr != nil {
			t.Fatalf("session %s should have been preserved: %v", sessionID, findErr)
		}
	}
}

func createCaptureForCleanupTest(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	accountID uuid.UUID,
	sessionID uuid.UUID,
	filename string,
	createdAt time.Time,
) capturedomain.Capture {
	t.Helper()
	item, err := capturedomain.NewCapture(capturedomain.NewCaptureParams{
		AccountID:        accountID,
		CaptureSessionID: sessionID,
		OriginalFilename: filename,
		MIMEType:         "application/pdf",
		SizeBytes:        128,
		CreatedAt:        createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err = item.ReserveStorageURI("supabase://captures/" + accountID.String() + "/" + item.ID.String() + ".pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateCapture(ctx, item); err != nil {
		t.Fatal(err)
	}
	return item
}

func createSessionForCleanupTest(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	accountID uuid.UUID,
	pairingHash []byte,
	createdAt time.Time,
) capturedomain.Session {
	t.Helper()
	session, err := capturedomain.NewSession(accountID, pairingHash, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	return session
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
	for _, migrationName := range []string{
		"20261009132454_create_capture_sessions_and_captures.sql",
		"20261009153157_reduce_capture_file_size_to_5_mib.sql",
	} {
		migrationPath := filepath.Join("..", "..", "..", "..", "supabase", "migrations", migrationName)
		migration, readErr := os.ReadFile(migrationPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		migrationSQL := strings.ReplaceAll(string(migration), "public.", identifier+".")
		if _, err = admin.Exec(ctx, migrationSQL); err != nil {
			t.Fatal(err)
		}
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
