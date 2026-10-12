// internal/infrastructure/documentai/text_extractor.go
package documentai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/documentai/apiv1/documentaipb"
	domaintext "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textreading"
)

type DocumentProcessor interface {
	ProcessDocument(context.Context, string, string, string) (*documentaipb.Document, error)
}

// TextExtractor uses the OCR text even when no structured lab entities were found.
type TextExtractor struct {
	client      DocumentProcessor
	processorID string
	timeout     time.Duration
}

var _ domaintext.Extractor = (*TextExtractor)(nil)

func NewTextExtractor(client DocumentProcessor, processorID string, timeout time.Duration) *TextExtractor {
	if timeout <= 0 {
		timeout = time.Minute
	}
	return &TextExtractor{client: client, processorID: processorID, timeout: timeout}
}

func (e *TextExtractor) Extract(ctx context.Context, input domaintext.ExtractInput) (*domaintext.ExtractOutput, error) {
	if e.client == nil || strings.TrimSpace(e.processorID) == "" {
		return nil, fmt.Errorf("document AI OCR is not configured")
	}
	if !strings.HasPrefix(input.DocumentURI, "gs://") {
		return nil, fmt.Errorf("document AI OCR requires a stored GCS document")
	}
	callCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	doc, err := e.client.ProcessDocument(callCtx, e.processorID, input.DocumentURI, input.MimeType)
	if err != nil {
		return nil, fmt.Errorf("document AI OCR failed: %w", err)
	}
	if doc == nil || strings.TrimSpace(doc.GetText()) == "" {
		return nil, fmt.Errorf("document AI returned no text")
	}
	return &domaintext.ExtractOutput{
		Text:           doc.GetText(),
		NormalizedText: domaintext.NormalizeForSemanticExtraction(doc.GetText()),
		Method:         "document_ai_ocr",
	}, nil
}
