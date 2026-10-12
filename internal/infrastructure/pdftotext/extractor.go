// internal/infrastructure/pdftotext/extractor.go
package pdftotext

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	textreading "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textreading"
)

const defaultTimeout = 20 * time.Second

type Extractor struct {
	timeout           time.Duration
	requireUsableText bool
}

type ExtractorOptions struct {
	Timeout           time.Duration
	RequireUsableText bool
}

var _ textreading.Extractor = (*Extractor)(nil)

func NewExtractor() *Extractor {
	return &Extractor{timeout: defaultTimeout, requireUsableText: true}
}

func NewExtractorWithOptions(options ExtractorOptions) *Extractor {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Extractor{timeout: timeout, requireUsableText: options.RequireUsableText}
}

func (e *Extractor) Extract(ctx context.Context, input textreading.ExtractInput) (*textreading.ExtractOutput, error) {
	if strings.TrimSpace(input.LocalPath) == "" {
		return nil, errors.New("local path is required")
	}
	switch normalizeMIMEType(input.MimeType) {
	case "application/pdf", "image/pdf":
	default:
		return nil, fmt.Errorf("unsupported MIME type for pdftotext: %s", input.MimeType)
	}

	text, err := e.run(ctx, input.LocalPath)
	if err != nil {
		return nil, err
	}
	if e.requireUsableText && !textreading.IsUsableText(text) {
		return nil, textreading.ErrUnreadablePDF
	}
	return &textreading.ExtractOutput{
		Text:           text,
		NormalizedText: textreading.NormalizeForSemanticExtraction(text),
		Method:         "pdf_text_raw",
	}, nil
}

func (e *Extractor) run(ctx context.Context, localPath string) (string, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", err
	}
	timeout := e.timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := exec.CommandContext(runCtx, "pdftotext", "-raw", localPath, "-").Output()
	if runCtx.Err() != nil {
		return "", runCtx.Err()
	}
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func normalizeMIMEType(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if index := strings.Index(normalized, ";"); index >= 0 {
		normalized = strings.TrimSpace(normalized[:index])
	}
	return normalized
}
