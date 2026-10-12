// internal/infrastructure/textextraction/command_extractor_test.go
package textextraction

import (
	"context"
	"errors"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	domaintext "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textextraction"
)

func TestInvalidPDFRemainsTechnicalFailure(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is required")
	}
	path := filepath.Join(t.TempDir(), "invalid.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\ninvalid content"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewCommandExtractor().extractPDF(context.Background(), path)
	if err == nil || errors.Is(err, domaintext.ErrUnreadablePDF) {
		t.Fatalf("invalid PDF must not trigger multimodal fallback: %v", err)
	}
}

func TestMissingPDFReaderRemainsTechnicalFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := NewCommandExtractor().extractPDF(context.Background(), "file.pdf")
	if err == nil || errors.Is(err, domaintext.ErrUnreadablePDF) {
		t.Fatalf("missing executable must remain a technical error: %v", err)
	}
}

func TestExpiredOCRDeadlineIsPreserved(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := NewCommandExtractor().extractImage(ctx, filepath.Join(t.TempDir(), "missing.jpg"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded instead of empty OCR text, got %v", err)
	}
}

func TestScoreOCRTextPrefersClinicalLabText(t *testing.T) {
	noisy := "meee eee rr iii eT an BOBIMDFTIOS 35"
	lab := "HEMOGRAMA COMPLETO Paciente OTAVIANO HEMOGLOBINA 14,6 g/dl PLAQUETAS 199000 /mm3"

	if scoreOCRText(lab) <= scoreOCRText(noisy) {
		t.Fatal("expected lab text to score higher than noisy OCR text")
	}
}

func TestScoreOCRTextPrefersPreservedLabPanelLines(t *testing.T) {
	noisy := "HEMOGRAMA HEMOGLOBINA 14,6 HEMATOCRITO 43,8 PLAQUETAS 199000"
	structured := "HEMOGRAMA\nHEMOGLOBINA........: 14,6 g/dl\nHEMATOCRITO........: 43,8 %\nPLAQUETAS..........: 199000 /mm3"

	if scoreOCRText(structured) <= scoreOCRText(noisy) {
		t.Fatal("expected preserved result lines to score higher")
	}
}

func TestOtsuThresholdSeparatesDarkAndLightPixels(t *testing.T) {
	var histogram [256]int
	histogram[20] = 50
	histogram[230] = 50

	threshold := otsuThreshold(histogram, 100)
	if threshold < 20 || threshold >= 230 {
		t.Fatalf("expected threshold between the two peaks, got %d", threshold)
	}
}

func TestOCRMethodDescribesSelectedVariation(t *testing.T) {
	got := ocrMethod(90, "enhanced", 6)
	want := "ocr_rotate_90_enhanced_psm_6"
	if got != want {
		t.Fatalf("ocrMethod() = %q, want %q", got, want)
	}
}

func TestResizeImageToMaxDimension(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 4000, 2000))
	resized := resizeImageToMaxDimension(source, 2200)

	if got, want := resized.Bounds().Dx(), 2200; got != want {
		t.Fatalf("width = %d, want %d", got, want)
	}
	if got, want := resized.Bounds().Dy(), 1100; got != want {
		t.Fatalf("height = %d, want %d", got, want)
	}
}

func TestRotationPattern(t *testing.T) {
	output := "Orientation in degrees: 90\nRotate: 270\nOrientation confidence: 3.31\n"
	matches := rotationPattern.FindStringSubmatch(output)
	if len(matches) != 2 || matches[1] != "270" {
		t.Fatalf("unexpected rotation matches: %#v", matches)
	}
}
