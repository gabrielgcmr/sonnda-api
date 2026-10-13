// internal/features/documentprocessing/temporary_page_file_test.go
package documentprocessing

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type temporaryCaptureSourceStub struct {
	accountID uuid.UUID
	captureID uuid.UUID
}

func (s temporaryCaptureSourceStub) OpenCaptureFile(_ context.Context, accountID, captureID uuid.UUID) (*capture.CaptureFile, error) {
	if accountID != s.accountID || captureID != s.captureID {
		return nil, apperr.NotFound("captura não encontrada")
	}
	return &capture.CaptureFile{Body: io.NopCloser(strings.NewReader("img"))}, nil
}

func (s temporaryCaptureSourceStub) GetCaptureFile(_ context.Context, accountID, captureID uuid.UUID) (*capture.SignedCaptureFile, error) {
	if accountID != s.accountID || captureID != s.captureID {
		return nil, apperr.NotFound("captura não encontrada")
	}
	return &capture.SignedCaptureFile{URL: "https://storage.test/capture", ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func TestTemporaryPageFileRequiresOwnerAndAvailablePage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exam.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0600); err != nil {
		t.Fatal(err)
	}
	repo, storage := &temporaryRepoStub{}, &temporaryStorageStub{}
	results := NewTemporaryResults(repo, storage)
	accountID := uuid.New()
	item, err := results.Enqueue(t.Context(), accountID, "laboratory", []TemporaryPage{{LocalPath: path, OriginalFilename: "exam.pdf", MIMEType: "application/pdf", SizeBytes: 8}})
	if err != nil {
		t.Fatal(err)
	}
	file, err := results.PageFile(t.Context(), accountID, item.ID, 0)
	if err != nil || file.URL != "https://storage.test/signed" || storage.signedTTL != temporaryPageURLLifetime {
		t.Fatalf("file=%+v ttl=%s error=%v", file, storage.signedTTL, err)
	}
	if _, err := results.PageFile(t.Context(), uuid.New(), item.ID, 0); !apperr.IsNotFound(err) {
		t.Fatalf("other account error=%v", err)
	}
	if _, err := results.PageFile(t.Context(), accountID, item.ID, 1); !apperr.IsNotFound(err) {
		t.Fatalf("missing page error=%v", err)
	}
	results.now = func() time.Time { return item.ExpiresAt.Add(-90 * time.Second) }
	if _, err := results.PageFile(t.Context(), accountID, item.ID, 0); err != nil || storage.signedTTL != 90*time.Second {
		t.Fatalf("URL outlives page: ttl=%s error=%v", storage.signedTTL, err)
	}
	results.now = func() time.Time { return item.ExpiresAt.Add(time.Second) }
	if _, err := results.PageFile(t.Context(), accountID, item.ID, 0); !apperr.IsNotFound(err) {
		t.Fatalf("expired page error=%v", err)
	}
}

func TestTemporaryPageFileUsesCaptureSignature(t *testing.T) {
	repo := &temporaryRepoStub{}
	results := NewTemporaryResults(repo, &temporaryStorageStub{})
	accountID, captureID := uuid.New(), uuid.New()
	results.SetCaptureSource(temporaryCaptureSourceStub{accountID: accountID, captureID: captureID})
	item, err := results.Enqueue(t.Context(), accountID, "imaging", []TemporaryPage{{CaptureID: &captureID, OriginalFilename: "foto.jpg",
		MIMEType: "image/jpeg", SizeBytes: 3, ExpiresAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	file, err := results.PageFile(t.Context(), accountID, item.ID, 0)
	if err != nil || file.URL != "https://storage.test/capture" {
		t.Fatalf("file=%+v error=%v", file, err)
	}
}
