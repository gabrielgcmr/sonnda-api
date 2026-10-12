// internal/infrastructure/documentai/text_extractor_test.go
package documentai

import (
	"context"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/documentai/apiv1/documentaipb"
	domaintext "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textreading"
)

type textProcessorFunc func(context.Context, string, string, string) (*documentaipb.Document, error)

func (f textProcessorFunc) ProcessDocument(ctx context.Context, processor, uri, mime string) (*documentaipb.Document, error) {
	return f(ctx, processor, uri, mime)
}

func TestTextExtractionDoesNotRequireLabEntities(t *testing.T) {
	raw := "Hematocrito 43,8 \uFF05"
	client := textProcessorFunc(func(ctx context.Context, processor, uri, mime string) (*documentaipb.Document, error) {
		if processor != "processor" || uri != "gs://bucket/photo.jpg" || mime != "image/jpeg" {
			t.Fatal("incorrect document request")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing deadline")
		}
		return &documentaipb.Document{Text: raw}, nil
	})
	out, err := NewTextExtractor(client, "processor", time.Minute).Extract(context.Background(), domaintext.ExtractInput{DocumentURI: "gs://bucket/photo.jpg", MimeType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != raw || out.NormalizedText != "Hematocrito 43,8 %" || out.Method != "document_ai_ocr" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestCloudOCRHonorsOwnAndParentDeadline(t *testing.T) {
	for _, parentExpires := range []bool{false, true} {
		ctx := context.Background()
		timeout := time.Millisecond
		if parentExpires {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Millisecond)
			defer cancel()
			timeout = time.Minute
		}
		client := textProcessorFunc(func(ctx context.Context, _, _, _ string) (*documentaipb.Document, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
		_, err := NewTextExtractor(client, "processor", timeout).Extract(ctx, domaintext.ExtractInput{DocumentURI: "gs://bucket/photo.jpg"})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected timeout, got %v", err)
		}
	}
}

func TestCloudOCRRejectsEmptyResponse(t *testing.T) {
	for _, doc := range []*documentaipb.Document{nil, {}, {Text: "  "}} {
		client := textProcessorFunc(func(context.Context, string, string, string) (*documentaipb.Document, error) { return doc, nil })
		_, err := NewTextExtractor(client, "processor", time.Minute).Extract(context.Background(), domaintext.ExtractInput{DocumentURI: "gs://bucket/photo.jpg"})
		if err == nil {
			t.Fatal("empty response accepted")
		}
	}
}
