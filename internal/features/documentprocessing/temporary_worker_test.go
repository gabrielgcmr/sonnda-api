// internal/features/documentprocessing/temporary_worker_test.go
package documentprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type temporaryAccountStub struct{ account *accountdomain.Account }

func (s temporaryAccountStub) FindByID(_ context.Context, id uuid.UUID) (*accountdomain.Account, error) {
	if s.account.ID != id {
		return nil, nil
	}
	return s.account, nil
}

type temporaryLabStub struct {
	fail     bool
	during   func(context.Context) error
	calls    int
	withTest bool
}

func (s *temporaryLabStub) ExtractDocument(ctx context.Context, _ *accountdomain.Account, path, _, _ string) (*labreport.Result, error) {
	s.calls++
	if s.during != nil {
		if err := s.during(ctx); err != nil {
			return nil, err
		}
	}
	if s.fail {
		return nil, apperr.Validation("O documento não pôde ser lido.")
	}
	if _, err := os.ReadFile(path); err != nil {
		return nil, err
	}
	result := &labreport.Result{Status: labreport.ExtractionStatusSucceeded, SummaryText: "Glicose: 90"}
	if s.withTest {
		result.Report.Tests = []labreport.ExtractedTestResult{{TestName: "Glicose", Items: []labreport.ExtractedTestItem{{ParameterName: "Glicose"}}}}
	}
	return result, nil
}

func TestTemporaryWorkerProcessesTwentyLaboratoryPagesWithOrigin(t *testing.T) {
	account := &accountdomain.Account{ID: uuid.New()}
	now := time.Now().UTC()
	storage := &temporaryStorageStub{objects: make(map[string][]byte)}
	pages := make([]TemporaryPage, 20)
	for index := range pages {
		uri := fmt.Sprintf("supabase://temporary-extractions/page-%d", index)
		storage.objects[uri] = []byte{0xff, 0xd8, 0xff}
		pages[index] = TemporaryPage{Position: index, StorageURI: uri, OriginalFilename: fmt.Sprintf("page-%d.jpg", index+1), MIMEType: "image/jpeg", SizeBytes: 3, ExpiresAt: now.Add(time.Hour)}
	}
	repo := &temporaryRepoStub{item: &TemporaryResult{ID: uuid.New(), AccountID: account.ID, Kind: "laboratory", Status: "queued", CreatedAt: now, ExpiresAt: now.Add(time.Hour), Pages: pages}}
	lab := &temporaryLabStub{withTest: true}
	worker := NewTemporaryWorker(NewTemporaryResults(repo, storage), temporaryAccountStub{account}, nil, lab, nil)
	if processed, err := worker.ProcessOne(t.Context()); err != nil || !processed {
		t.Fatalf("processed=%v error=%v", processed, err)
	}
	if lab.calls != 20 || repo.item.Status != "completed" || repo.item.Progress != 100 {
		t.Fatalf("calls=%d status=%s progress=%d", lab.calls, repo.item.Status, repo.item.Progress)
	}
	var result labreport.Result
	if err := json.Unmarshal(repo.item.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Report.Tests) != 20 || result.Report.Tests[19].SourcePage != 20 {
		t.Fatalf("page origin lost: %+v", result.Report.Tests)
	}
}

func TestTemporaryWorkerPersistsResultAfterRequestEnds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exam.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	repo, storage := &temporaryRepoStub{}, &temporaryStorageStub{}
	results := NewTemporaryResults(repo, storage)
	account := &accountdomain.Account{ID: uuid.New()}
	item, err := results.Enqueue(t.Context(), account.ID, "laboratory", []TemporaryPage{{LocalPath: path, OriginalFilename: "exam.pdf", MIMEType: "application/pdf", SizeBytes: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "queued" || len(item.Result) != 0 {
		t.Fatalf("queued item = %+v", item)
	}
	// A new service instance models a request/browser lifetime ending before work starts.
	worker := NewTemporaryWorker(results, temporaryAccountStub{account}, nil, &temporaryLabStub{}, nil)
	processed, err := worker.ProcessOne(t.Context())
	if err != nil || !processed {
		t.Fatalf("processed=%v error=%v", processed, err)
	}
	otherInstance := NewTemporaryResults(repo, storage)
	saved, err := otherInstance.Get(t.Context(), account.ID, item.ID)
	if err != nil || saved.Status != "completed" || saved.Progress != 100 || len(saved.Result) == 0 {
		t.Fatalf("saved=%+v error=%v", saved, err)
	}
	if _, err := otherInstance.Get(t.Context(), uuid.New(), item.ID); err == nil {
		t.Fatal("other account accessed the result")
	}
}

func enqueueTemporaryLab(t *testing.T) (*temporaryRepoStub, *TemporaryResults, *accountdomain.Account) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exam.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	repo := &temporaryRepoStub{}
	results := NewTemporaryResults(repo, &temporaryStorageStub{})
	account := &accountdomain.Account{ID: uuid.New()}
	if _, err := results.Enqueue(t.Context(), account.ID, "laboratory", []TemporaryPage{{LocalPath: path, OriginalFilename: "exam.pdf", MIMEType: "application/pdf", SizeBytes: 8}}); err != nil {
		t.Fatal(err)
	}
	return repo, results, account
}

func TestTemporaryWorkerRecordsExtractorPanicAsFailure(t *testing.T) {
	repo, results, account := enqueueTemporaryLab(t)
	lab := &temporaryLabStub{during: func(context.Context) error { panic("extractor bug") }}
	worker := NewTemporaryWorker(results, temporaryAccountStub{account}, nil, lab, nil)
	if _, err := worker.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if repo.item.Status != "failed" || repo.item.ErrorMessage != temporaryFailureMessage {
		t.Fatalf("panic not recorded: %+v", repo.item)
	}
}

func TestTemporaryWorkerFailsWhenSourcesExpireBeforeCompletion(t *testing.T) {
	repo, results, account := enqueueTemporaryLab(t)
	lab := &temporaryLabStub{during: func(context.Context) error {
		repo.item.ExpiresAt = time.Now().Add(-time.Second)
		return nil
	}}
	worker := NewTemporaryWorker(results, temporaryAccountStub{account}, nil, lab, nil)
	if _, err := worker.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if repo.item.Status != "failed" || repo.item.ErrorMessage != temporaryExpiredMessage || len(repo.item.Result) != 0 {
		t.Fatalf("expired sources not recorded: %+v", repo.item)
	}
}

func TestTemporaryWorkerReleasesJobOnShutdown(t *testing.T) {
	repo, results, account := enqueueTemporaryLab(t)
	ctx, cancel := context.WithCancel(t.Context())
	lab := &temporaryLabStub{during: func(jobCtx context.Context) error {
		cancel()
		return jobCtx.Err()
	}}
	worker := NewTemporaryWorker(results, temporaryAccountStub{account}, nil, lab, nil)
	if _, err := worker.ProcessOne(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if repo.item.Status != "queued" || repo.owner != uuid.Nil {
		t.Fatalf("job not released: %+v", repo.item)
	}
}

func TestTemporaryWorkerFailureCanRetryWhileSourceExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exam.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	repo, storage := &temporaryRepoStub{}, &temporaryStorageStub{}
	results := NewTemporaryResults(repo, storage)
	account := &accountdomain.Account{ID: uuid.New()}
	item, err := results.Enqueue(t.Context(), account.ID, "laboratory", []TemporaryPage{{LocalPath: path, OriginalFilename: "exam.pdf", MIMEType: "application/pdf", SizeBytes: 8}})
	if err != nil {
		t.Fatal(err)
	}
	lab := &temporaryLabStub{fail: true}
	worker := NewTemporaryWorker(results, temporaryAccountStub{account}, nil, lab, nil)
	if _, err := worker.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if repo.item.Status != "failed" || repo.item.ErrorMessage == "" {
		t.Fatalf("failure not persisted: %+v", repo.item)
	}
	if _, err := results.Retry(t.Context(), uuid.New(), item.ID); err == nil {
		t.Fatal("other account retried work")
	}
	if _, err := results.Retry(t.Context(), account.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	lab.fail = false
	if _, err := worker.ProcessOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if repo.item.Status != "completed" {
		t.Fatalf("retry status=%s", repo.item.Status)
	}
	repo.item.Status = "failed"
	delete(storage.objects, repo.item.Pages[0].StorageURI)
	if _, err := results.Retry(t.Context(), account.ID, item.ID); err == nil || !errors.As(err, new(*apperr.AppError)) {
		t.Fatalf("missing source retry error=%v", err)
	}
	results.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	if _, err := results.Get(t.Context(), account.ID, item.ID); err == nil {
		t.Fatal("expired work remained available")
	}
}
