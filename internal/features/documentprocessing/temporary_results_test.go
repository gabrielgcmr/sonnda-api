// internal/features/documentprocessing/temporary_results_test.go
package documentprocessing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type temporaryRepoStub struct {
	item      *TemporaryResult
	createErr error
	deleted   bool
	owner     uuid.UUID
}

func (r *temporaryRepoStub) Create(_ context.Context, item *TemporaryResult) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.item = item
	return nil
}
func (r *temporaryRepoStub) List(_ context.Context, accountID uuid.UUID, now time.Time) ([]TemporaryResult, error) {
	if r.item != nil && r.item.AccountID == accountID && r.item.ExpiresAt.After(now) {
		return []TemporaryResult{*r.item}, nil
	}
	return []TemporaryResult{}, nil
}
func (r *temporaryRepoStub) Get(_ context.Context, accountID, id uuid.UUID, now time.Time) (*TemporaryResult, error) {
	if r.item != nil && r.item.AccountID == accountID && r.item.ID == id && r.item.ExpiresAt.After(now) {
		return r.item, nil
	}
	return nil, nil
}
func (r *temporaryRepoStub) Expired(_ context.Context, now time.Time, _ int) ([]TemporaryResult, error) {
	if r.item != nil && !r.deleted && !r.item.ExpiresAt.After(now) {
		return []TemporaryResult{*r.item}, nil
	}
	return nil, nil
}
func (r *temporaryRepoStub) Delete(_ context.Context, _ uuid.UUID) error {
	r.deleted = true
	return nil
}
func (r *temporaryRepoStub) Claim(_ context.Context, owner uuid.UUID, now time.Time, _ time.Duration, _ int, _ string) (*TemporaryResult, error) {
	if r.item == nil || r.item.Status != "queued" || !r.item.ExpiresAt.After(now) {
		return nil, nil
	}
	r.item.Status, r.item.Progress, r.owner = "processing", 10, owner
	return r.item, nil
}
func (r *temporaryRepoStub) Release(_ context.Context, id, owner uuid.UUID) (bool, error) {
	if r.item == nil || r.item.ID != id || r.owner != owner || r.item.Status != "processing" {
		return false, nil
	}
	r.item.Status, r.item.Progress, r.owner = "queued", 0, uuid.Nil
	return true, nil
}
func (r *temporaryRepoStub) Renew(_ context.Context, id, owner uuid.UUID, _ time.Time, _ time.Duration) (bool, error) {
	return r.item != nil && r.item.ID == id && r.owner == owner, nil
}
func (r *temporaryRepoStub) Advance(_ context.Context, id, owner uuid.UUID, _ time.Time, progress int) (bool, error) {
	if r.item == nil || r.item.ID != id || r.owner != owner {
		return false, nil
	}
	r.item.Progress = progress
	return true, nil
}
func (r *temporaryRepoStub) Complete(_ context.Context, id, owner uuid.UUID, now time.Time, result json.RawMessage) (bool, error) {
	if r.item == nil || r.item.ID != id || r.owner != owner || !r.item.ExpiresAt.After(now) {
		return false, nil
	}
	r.item.Status, r.item.Progress, r.item.Result = "completed", 100, result
	return true, nil
}
func (r *temporaryRepoStub) Fail(_ context.Context, id, owner uuid.UUID, _ time.Time, message string) (bool, error) {
	if r.item == nil || r.item.ID != id || r.owner != owner {
		return false, nil
	}
	r.item.Status, r.item.ErrorMessage = "failed", message
	return true, nil
}
func (r *temporaryRepoStub) Retry(_ context.Context, accountID, id uuid.UUID, now time.Time) (bool, error) {
	if r.item == nil || r.item.ID != id || r.item.AccountID != accountID || r.item.Status != "failed" || !r.item.ExpiresAt.After(now) {
		return false, nil
	}
	r.item.Status, r.item.ErrorMessage, r.item.Progress = "queued", "", 0
	return true, nil
}

type temporaryStorageStub struct {
	uploads   int
	deleted   []string
	objects   map[string][]byte
	signedTTL time.Duration
}

func (s *temporaryStorageStub) Upload(_ context.Context, source io.Reader, _ string, _ string) (string, error) {
	data, err := io.ReadAll(source)
	if err != nil {
		return "", err
	}
	s.uploads++
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects["supabase://temporary-extractions/test"] = data
	return "supabase://temporary-extractions/test", nil
}
func (s *temporaryStorageStub) Delete(_ context.Context, uri string) error {
	s.deleted = append(s.deleted, uri)
	delete(s.objects, uri)
	return nil
}
func (s *temporaryStorageStub) GetSignedURL(_ context.Context, uri string, expiresIn time.Duration) (string, error) {
	if _, ok := s.objects[uri]; !ok {
		return "", apperr.NotFound("object missing")
	}
	s.signedTTL = expiresIn
	return "https://storage.test/signed", nil
}
func (s *temporaryStorageStub) Open(_ context.Context, uri string) (io.ReadCloser, error) {
	data, ok := s.objects[uri]
	if !ok {
		return nil, errors.New("object missing")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func TestTemporaryResultSurvivesLocalStateAndExpiresWithItsFile(t *testing.T) {
	ctx := t.Context()
	base := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "exam.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	repo, storage := &temporaryRepoStub{}, &temporaryStorageStub{}
	service := NewTemporaryResults(repo, storage)
	service.now = func() time.Time { return base }
	accountID := uuid.New()
	item, err := service.Save(ctx, accountID, "laboratory", map[string]string{"summary_text": "Glicose: 90"},
		[]TemporaryPage{{LocalPath: path, OriginalFilename: "exam.pdf", MIMEType: "application/pdf", SizeBytes: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if storage.uploads != 1 || !item.ExpiresAt.Equal(base.Add(24*time.Hour)) {
		t.Fatalf("upload=%d, expiry=%v", storage.uploads, item.ExpiresAt)
	}
	if _, err := service.Get(ctx, accountID, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, uuid.New(), item.ID); err == nil {
		t.Fatal("another account accessed the result")
	}
	service.now = func() time.Time { return base.Add(24 * time.Hour) }
	if _, err := service.Get(ctx, accountID, item.ID); err == nil {
		t.Fatal("expired result remained available")
	}
	if err := service.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if !repo.deleted || len(storage.deleted) != 1 {
		t.Fatalf("cleanup: deleted=%v, objects=%v", repo.deleted, storage.deleted)
	}
}

func TestTemporaryResultUsesCaptureExpiryAndDoesNotUploadAgain(t *testing.T) {
	base := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	repo, storage := &temporaryRepoStub{}, &temporaryStorageStub{}
	service := NewTemporaryResults(repo, storage)
	service.now = func() time.Time { return base }
	id := uuid.New()
	expires := base.Add(2 * time.Hour)
	item, err := service.Save(t.Context(), uuid.New(), "laboratory", map[string]string{"status": "succeeded"},
		[]TemporaryPage{{CaptureID: &id, OriginalFilename: "foto.jpg", MIMEType: "image/jpeg", SizeBytes: 100, ExpiresAt: expires}})
	if err != nil {
		t.Fatal(err)
	}
	if storage.uploads != 0 || !item.ExpiresAt.Equal(expires) {
		t.Fatalf("capture was copied or expiry changed: uploads=%d, expiry=%v", storage.uploads, item.ExpiresAt)
	}
}

func TestTemporaryResultRemovesUploadedFileWhenDatabaseFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exam.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	repo := &temporaryRepoStub{createErr: errors.New("database unavailable")}
	storage := &temporaryStorageStub{}
	service := NewTemporaryResults(repo, storage)
	_, err := service.Save(t.Context(), uuid.New(), "laboratory", map[string]string{"status": "succeeded"},
		[]TemporaryPage{{LocalPath: path, OriginalFilename: "exam.pdf", MIMEType: "application/pdf", SizeBytes: 8}})
	if err == nil || len(storage.deleted) != 1 {
		t.Fatalf("error=%v, deleted=%v", err, storage.deleted)
	}
}
