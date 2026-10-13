// internal/features/documentprocessing/postgres/temporary_results_integration_test.go
//go:build integration

package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	processing "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	pginfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const temporaryTestExhausted = "esgotado"

func temporaryTestDatabase(t *testing.T) (*pginfra.Client, *TemporaryResultRepository, uuid.UUID, uuid.UUID) {
	t.Helper()
	rawURL := os.Getenv("TEMPORARY_EXTRACTIONS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set TEMPORARY_EXTRACTIONS_TEST_DATABASE_URL to an isolated local Supabase Postgres")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("integration tests require a local Supabase Postgres URL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	schema := "temporary_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	if _, err := admin.Exec(ctx, `DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='anon') THEN CREATE ROLE anon; END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN CREATE ROLE authenticated; END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='service_role') THEN CREATE ROLE service_role; END IF;
 END $$;`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE TABLE "+identifier+".accounts (id uuid PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"20261009132454_create_capture_sessions_and_captures.sql",
		"20261009153157_reduce_capture_file_size_to_5_mib.sql",
		"20261010022320_add_capture_cleanup_run_observability.sql",
		"20261013004010_temporary_extraction_results.sql",
		"20261013011351_temporary_extraction_jobs.sql",
	} {
		migration, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "supabase", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, strings.ReplaceAll(string(migration), "public.", identifier+".")); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	params := u.Query()
	params.Set("search_path", schema)
	u.RawQuery = params.Encode()
	client, err := pginfra.NewClient(pginfra.Config{DatabaseURL: u.String(), MaxConns: 10})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	accountID, otherAccountID := uuid.New(), uuid.New()
	if _, err := client.Pool().Exec(ctx, "INSERT INTO accounts(id) VALUES ($1),($2)", accountID, otherAccountID); err != nil {
		t.Fatal(err)
	}
	return client, NewTemporaryResultRepository(client), accountID, otherAccountID
}

func createTemporaryJob(t *testing.T, repo *TemporaryResultRepository, accountID uuid.UUID, now time.Time, pages int) uuid.UUID {
	t.Helper()
	item := &processing.TemporaryResult{ID: uuid.New(), AccountID: accountID, Kind: "imaging", Status: "queued",
		CreatedAt: now, ExpiresAt: now.Add(processing.TemporaryRetention)}
	for i := range pages {
		item.Pages = append(item.Pages, processing.TemporaryPage{Position: i, StorageURI: "supabase://temporary-extractions/" + item.ID.String() + "/" + string(rune('a'+i)),
			OriginalFilename: "page.jpg", MIMEType: "image/jpeg", SizeBytes: 100, ExpiresAt: item.ExpiresAt})
	}
	if err := repo.Create(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	return item.ID
}

func temporaryJobState(t *testing.T, client *pginfra.Client, id uuid.UUID) (status string, attempts int, owner *uuid.UUID, message *string) {
	t.Helper()
	err := client.Pool().QueryRow(context.Background(), "SELECT status,attempts,lease_owner,error_message FROM temporary_extractions WHERE id=$1", id).
		Scan(&status, &attempts, &owner, &message)
	if err != nil {
		t.Fatal(err)
	}
	return status, attempts, owner, message
}

func TestTemporaryClaimIsExclusiveUnderConcurrency(t *testing.T) {
	client, repo, account, _ := temporaryTestDatabase(t)
	now := time.Now().UTC()
	id := createTemporaryJob(t, repo, account, now, 1)
	var wg sync.WaitGroup
	claimed := make(chan uuid.UUID, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item, err := repo.Claim(context.Background(), uuid.New(), now, 2*time.Minute, 3, temporaryTestExhausted)
			if err != nil {
				failures <- err
				return
			}
			if item != nil {
				claimed <- item.ID
			}
		}()
	}
	wg.Wait()
	close(claimed)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claims=%d, want 1", len(claimed))
	}
	if got := <-claimed; got != id {
		t.Fatalf("claimed %s, want %s", got, id)
	}
	if status, attempts, _, _ := temporaryJobState(t, client, id); status != "processing" || attempts != 1 {
		t.Fatalf("status=%s attempts=%d", status, attempts)
	}
}

func TestTemporaryExpiredLeaseIsReclaimedAndOldOwnerLosesWrites(t *testing.T) {
	_, repo, account, _ := temporaryTestDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id := createTemporaryJob(t, repo, account, now, 2)
	first, second := uuid.New(), uuid.New()
	if item, err := repo.Claim(ctx, first, now, 2*time.Minute, 3, temporaryTestExhausted); err != nil || item == nil || len(item.Pages) != 2 {
		t.Fatalf("first claim=%+v error=%v", item, err)
	}
	if item, err := repo.Claim(ctx, second, now.Add(time.Minute), 2*time.Minute, 3, temporaryTestExhausted); err != nil || item != nil {
		t.Fatalf("valid lease stolen: %+v %v", item, err)
	}
	if ok, err := repo.Renew(ctx, id, first, now.Add(time.Minute), 2*time.Minute); err != nil || !ok {
		t.Fatalf("renew=%v error=%v", ok, err)
	}
	later := now.Add(4 * time.Minute)
	if item, err := repo.Claim(ctx, second, later, 2*time.Minute, 3, temporaryTestExhausted); err != nil || item == nil || item.ID != id {
		t.Fatalf("expired lease not reclaimed: %+v %v", item, err)
	}
	result := json.RawMessage(`{"summary_text":"ok","report":{"patient_name":" Maria Souza ","exam_name":"USG abdome"}}`)
	for name, write := range map[string]func() (bool, error){
		"renew":    func() (bool, error) { return repo.Renew(ctx, id, first, later, 2*time.Minute) },
		"advance":  func() (bool, error) { return repo.Advance(ctx, id, first, later, 50) },
		"complete": func() (bool, error) { return repo.Complete(ctx, id, first, later, result) },
		"fail":     func() (bool, error) { return repo.Fail(ctx, id, first, later, "x") },
		"release":  func() (bool, error) { return repo.Release(ctx, id, first) },
	} {
		if ok, err := write(); err != nil || ok {
			t.Fatalf("old owner %s=%v error=%v", name, ok, err)
		}
	}
	if ok, err := repo.Complete(ctx, id, second, later, result); err != nil || !ok {
		t.Fatalf("complete=%v error=%v", ok, err)
	}
	saved, err := repo.Get(ctx, account, id, later)
	if err != nil || saved == nil || saved.Status != "completed" || saved.Progress != 100 || !json.Valid(saved.Result) {
		t.Fatalf("saved=%+v error=%v", saved, err)
	}
	items, err := repo.List(ctx, account, later)
	if err != nil || len(items) != 1 || items[0].PatientName != "Maria Souza" || items[0].ExamName != "USG abdome" {
		t.Fatalf("list=%+v error=%v", items, err)
	}
}

func TestTemporaryExhaustedAttemptsFailAndRetryResetsBudget(t *testing.T) {
	client, repo, account, other := temporaryTestDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id := createTemporaryJob(t, repo, account, now, 1)
	for attempt := range 3 {
		at := now.Add(time.Duration(attempt*3) * time.Minute)
		if item, err := repo.Claim(ctx, uuid.New(), at, 2*time.Minute, 3, temporaryTestExhausted); err != nil || item == nil {
			t.Fatalf("attempt %d claim=%+v error=%v", attempt+1, item, err)
		}
	}
	after := now.Add(12 * time.Minute)
	if item, err := repo.Claim(ctx, uuid.New(), after, 2*time.Minute, 3, temporaryTestExhausted); err != nil || item != nil {
		t.Fatalf("exhausted job claimed: %+v %v", item, err)
	}
	status, attempts, owner, message := temporaryJobState(t, client, id)
	if status != "failed" || attempts != 3 || owner != nil || message == nil || *message != temporaryTestExhausted {
		t.Fatalf("status=%s attempts=%d owner=%v message=%v", status, attempts, owner, message)
	}
	if ok, err := repo.Retry(ctx, other, id, after); err != nil || ok {
		t.Fatalf("other account retried: %v %v", ok, err)
	}
	if ok, err := repo.Retry(ctx, account, id, after); err != nil || !ok {
		t.Fatalf("retry=%v error=%v", ok, err)
	}
	if status, attempts, _, message := temporaryJobState(t, client, id); status != "queued" || attempts != 0 || message != nil {
		t.Fatalf("after retry status=%s attempts=%d message=%v", status, attempts, message)
	}
	if item, err := repo.Claim(ctx, uuid.New(), after, 2*time.Minute, 3, temporaryTestExhausted); err != nil || item == nil {
		t.Fatalf("retried job not claimable: %+v %v", item, err)
	}
}

func TestTemporaryCompleteAndRetryRefuseExpiredPage(t *testing.T) {
	client, repo, account, _ := temporaryTestDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id := createTemporaryJob(t, repo, account, now, 2)
	owner := uuid.New()
	if item, err := repo.Claim(ctx, owner, now, 2*time.Minute, 3, temporaryTestExhausted); err != nil || item == nil {
		t.Fatalf("claim=%+v error=%v", item, err)
	}
	if _, err := client.Pool().Exec(ctx, "UPDATE temporary_extraction_pages SET expires_at=$2 WHERE extraction_id=$1 AND position=1", id, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.Complete(ctx, id, owner, now, json.RawMessage(`{}`)); err != nil || ok {
		t.Fatalf("completed with expired page: %v %v", ok, err)
	}
	if ok, err := repo.Fail(ctx, id, owner, now, "expirado"); err != nil || !ok {
		t.Fatalf("fail=%v error=%v", ok, err)
	}
	if ok, err := repo.Retry(ctx, account, id, now); err != nil || ok {
		t.Fatalf("retried with expired page: %v %v", ok, err)
	}
}

type memoryTemporaryStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (s *memoryTemporaryStorage) Upload(_ context.Context, source io.Reader, path, _ string) (string, error) {
	data, err := io.ReadAll(source)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	uri := "supabase://temporary-extractions/" + path
	s.objects[uri] = data
	return uri, nil
}

func (s *memoryTemporaryStorage) Open(_ context.Context, uri string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[uri]
	if !ok {
		return nil, errors.New("object missing")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *memoryTemporaryStorage) GetSignedURL(_ context.Context, uri string, _ time.Duration) (string, error) {
	return "https://storage.test/" + uri, nil
}

func (s *memoryTemporaryStorage) Delete(_ context.Context, uri string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, uri)
	return nil
}

type fixedTemporaryAccount struct{ account *accountdomain.Account }

func (f fixedTemporaryAccount) FindByID(_ context.Context, id uuid.UUID) (*accountdomain.Account, error) {
	if f.account.ID != id {
		return nil, nil
	}
	return f.account, nil
}

type blockingTemporaryLab struct{ started chan struct{} }

func (b blockingTemporaryLab) ExtractDocument(ctx context.Context, _ *accountdomain.Account, _, _, _ string) (*labreport.Result, error) {
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

type readyTemporaryLab struct{}

func (readyTemporaryLab) ExtractDocument(_ context.Context, _ *accountdomain.Account, path, _, _ string) (*labreport.Result, error) {
	if _, err := os.ReadFile(path); err != nil {
		return nil, err
	}
	return &labreport.Result{Status: labreport.ExtractionStatusSucceeded, SummaryText: "Glicose: 90"}, nil
}

func TestTemporaryWorkerResumesAfterShutdownAndCrash(t *testing.T) {
	client, repo, accountID, _ := temporaryTestDatabase(t)
	ctx := context.Background()
	account := &accountdomain.Account{ID: accountID}
	results := processing.NewTemporaryResults(repo, &memoryTemporaryStorage{})
	path := filepath.Join(t.TempDir(), "exam.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	item, err := results.Enqueue(ctx, accountID, "laboratory", []processing.TemporaryPage{{LocalPath: path, OriginalFilename: "exam.pdf", MIMEType: "application/pdf", SizeBytes: 8}})
	if err != nil {
		t.Fatal(err)
	}

	// Graceful shutdown in the middle of extraction hands the job back immediately.
	lab := blockingTemporaryLab{started: make(chan struct{})}
	stopping := processing.NewTemporaryWorker(results, fixedTemporaryAccount{account}, nil, lab, nil)
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := stopping.ProcessOne(workerCtx)
		done <- err
	}()
	select {
	case <-lab.started:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not start extraction")
	}
	if saved, err := results.Get(ctx, accountID, item.ID); err != nil || saved.Status != "processing" || saved.Progress != 30 {
		t.Fatalf("in-progress state=%+v error=%v", saved, err)
	}
	stop()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown error=%v", err)
	}
	if status, attempts, owner, _ := temporaryJobState(t, client, item.ID); status != "queued" || attempts != 0 || owner != nil {
		t.Fatalf("after shutdown status=%s attempts=%d owner=%v", status, attempts, owner)
	}

	// A crashed instance keeps its reservation until the lease lapses.
	if claimed, err := repo.Claim(ctx, uuid.New(), time.Now().UTC(), 2*time.Minute, 3, temporaryTestExhausted); err != nil || claimed == nil {
		t.Fatalf("crashed claim=%+v error=%v", claimed, err)
	}
	survivor := processing.NewTemporaryWorker(results, fixedTemporaryAccount{account}, nil, readyTemporaryLab{}, nil)
	if processed, err := survivor.ProcessOne(ctx); err != nil || processed {
		t.Fatalf("valid lease taken: processed=%v error=%v", processed, err)
	}
	if _, err := client.Pool().Exec(ctx, "UPDATE temporary_extractions SET lease_until=now()-interval '1 second' WHERE id=$1", item.ID); err != nil {
		t.Fatal(err)
	}
	if processed, err := survivor.ProcessOne(ctx); err != nil || !processed {
		t.Fatalf("resume processed=%v error=%v", processed, err)
	}
	saved, err := results.Get(ctx, accountID, item.ID)
	if err != nil || saved.Status != "completed" || saved.Progress != 100 || !json.Valid(saved.Result) {
		t.Fatalf("resumed result=%+v error=%v", saved, err)
	}
	if _, attempts, _, _ := temporaryJobState(t, client, item.ID); attempts != 2 {
		t.Fatalf("attempts=%d, want 2", attempts)
	}
}

func TestTemporaryReleaseKeepsAttemptBudgetAndAccountsStayIsolated(t *testing.T) {
	client, repo, account, other := temporaryTestDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id := createTemporaryJob(t, repo, account, now, 1)
	owner := uuid.New()
	if item, err := repo.Claim(ctx, owner, now, 2*time.Minute, 3, temporaryTestExhausted); err != nil || item == nil {
		t.Fatalf("claim=%+v error=%v", item, err)
	}
	if ok, err := repo.Release(ctx, id, uuid.New()); err != nil || ok {
		t.Fatalf("foreign release=%v error=%v", ok, err)
	}
	if ok, err := repo.Release(ctx, id, owner); err != nil || !ok {
		t.Fatalf("release=%v error=%v", ok, err)
	}
	if status, attempts, leaseOwner, _ := temporaryJobState(t, client, id); status != "queued" || attempts != 0 || leaseOwner != nil {
		t.Fatalf("status=%s attempts=%d owner=%v", status, attempts, leaseOwner)
	}
	if item, err := repo.Claim(ctx, uuid.New(), now, 2*time.Minute, 3, temporaryTestExhausted); err != nil || item == nil {
		t.Fatalf("released job not claimable: %+v %v", item, err)
	}
	if item, err := repo.Get(ctx, other, id, now); err != nil || item != nil {
		t.Fatalf("other account read job: %+v %v", item, err)
	}
	if items, err := repo.List(ctx, other, now); err != nil || len(items) != 0 {
		t.Fatalf("other account listed jobs: %d %v", len(items), err)
	}
	if items, err := repo.List(ctx, account, now); err != nil || len(items) != 1 || items[0].Status != "processing" {
		t.Fatalf("owner list=%+v error=%v", items, err)
	}
}
