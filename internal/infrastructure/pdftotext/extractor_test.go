// internal/infrastructure/pdftotext/extractor_test.go
package pdftotext

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	textreading "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textreading"
)

func TestInvalidPDFRemainsTechnicalFailure(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is required")
	}
	path := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\ninvalid content"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewExtractor().Extract(context.Background(), textreading.ExtractInput{LocalPath: path, MimeType: "application/pdf"})
	if err == nil || errors.Is(err, textreading.ErrUnreadablePDF) {
		t.Fatalf("invalid PDF must remain a technical failure: %v", err)
	}
}

func TestMissingPDFReaderRemainsTechnicalFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := NewExtractor().Extract(context.Background(), textreading.ExtractInput{LocalPath: "file.pdf", MimeType: "application/pdf"})
	if err == nil || errors.Is(err, textreading.ErrUnreadablePDF) {
		t.Fatalf("missing executable must remain a technical failure: %v", err)
	}
}

