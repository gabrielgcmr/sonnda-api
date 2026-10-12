// internal/infrastructure/gemini/client_test.go
package gemini

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/config"
	"google.golang.org/genai"
)

type generatorFunc func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)

func (f generatorFunc) GenerateContent(ctx context.Context, model string, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return f(ctx, model, contents, cfg)
}

func testConfig() config.GeminiConfig {
	return config.GeminiConfig{Model: "test-model", Timeout: time.Second, MaxInputBytes: 1024, MaxMediaBytes: 1024, MaxOutputTokens: 256}
}

func TestClientConfigurationAndRequest(t *testing.T) {
	request := GenerateRequest{
		Text: " Glicose 99 mg/dL\n", SystemInstruction: "Retorne JSON.",
		JSONSchema: map[string]any{"type": "object"},
	}
	response := &genai.GenerateContentResponse{}
	called := false
	client, err := NewClientWithGenerator(testConfig(), generatorFunc(func(ctx context.Context, model string, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		called = true
		if model != "test-model" || cfg.MaxOutputTokens != 256 || cfg.CandidateCount != 1 {
			t.Fatal("model or generation limits were not passed to SDK")
		}
		if len(contents) != 1 || contents[0].Parts[0].Text != request.Text {
			t.Fatal("input text was changed or truncated")
		}
		if cfg.SystemInstruction.Parts[0].Text != request.SystemInstruction || cfg.ResponseMIMEType != "application/json" || !reflect.DeepEqual(cfg.ResponseJsonSchema, request.JSONSchema) {
			t.Fatal("instructions or schema were not passed to SDK")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("generation has no deadline")
		}
		return response, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Generate(context.Background(), request)
	if err != nil || got != response || !called {
		t.Fatalf("unexpected generation result: %v", err)
	}
}

func TestClientMultimodalRequest(t *testing.T) {
	media := &MediaInput{Data: []byte{0xff, 0xd8, 0xff, 0xe0}, MIMEType: " IMAGE/JPEG "}
	client, err := NewClientWithGenerator(testConfig(), generatorFunc(func(_ context.Context, _ string, contents []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		if len(contents) != 1 || len(contents[0].Parts) != 2 {
			t.Fatalf("unexpected multimodal contents: %+v", contents)
		}
		if contents[0].Parts[0].Text != "Extraia o documento." {
			t.Fatalf("unexpected prompt: %q", contents[0].Parts[0].Text)
		}
		blob := contents[0].Parts[1].InlineData
		if blob == nil || blob.MIMEType != "image/jpeg" || !reflect.DeepEqual(blob.Data, media.Data) {
			t.Fatalf("unexpected inline media: %+v", blob)
		}
		return &genai.GenerateContentResponse{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Generate(t.Context(), GenerateRequest{Text: "Extraia o documento.", Media: media}); err != nil {
		t.Fatal(err)
	}
}

func TestClientSendsMultipleImagesInPageOrder(t *testing.T) {
	first := []byte{0xff, 0xd8, 0xff, 0xe0, 0x01}
	second := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x02}
	client, err := NewClientWithGenerator(testConfig(), generatorFunc(func(_ context.Context, _ string, contents []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		if len(contents) != 1 || len(contents[0].Parts) != 2 {
			t.Fatalf("expected two ordered image parts: %+v", contents)
		}
		parts := contents[0].Parts
		if parts[0].InlineData.MIMEType != "image/jpeg" || !reflect.DeepEqual(parts[0].InlineData.Data, first) ||
			parts[1].InlineData.MIMEType != "image/png" || !reflect.DeepEqual(parts[1].InlineData.Data, second) {
			t.Fatalf("image parts were reordered: %+v", parts)
		}
		return &genai.GenerateContentResponse{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Generate(t.Context(), GenerateRequest{MediaParts: []MediaInput{{Data: first, MIMEType: "image/jpeg"}, {Data: second, MIMEType: "image/png"}}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientAcceptsSupportedMediaSignatures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mimeType string
		data     []byte
	}{
		{name: "PDF", mimeType: "application/pdf", data: []byte("%PDF-1.7")},
		{name: "JPEG", mimeType: "image/jpeg", data: []byte{0xff, 0xd8, 0xff, 0xe0}},
		{name: "PNG", mimeType: "image/png", data: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClientWithGenerator(testConfig(), generatorFunc(func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
				return &genai.GenerateContentResponse{}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Generate(t.Context(), GenerateRequest{Media: &MediaInput{Data: tc.data, MIMEType: tc.mimeType}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientRejectsInputBeforeCallingSDK(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request GenerateRequest
		limit   int
		want    error
	}{
		{"empty", GenerateRequest{Text: " \n"}, 100, ErrEmptyInput},
		{"text_limit", GenerateRequest{Text: "12345"}, 4, ErrInputTooLarge},
		{"utf8_bytes", GenerateRequest{Text: "\u00e1"}, 1, ErrInputTooLarge},
		{"instruction_limit", GenerateRequest{Text: "123", SystemInstruction: "45"}, 4, ErrInputTooLarge},
		{"schema_limit", GenerateRequest{Text: "1", JSONSchema: map[string]any{"type": "object"}}, 4, ErrInputTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.MaxInputBytes = tc.limit
			client, err := NewClientWithGenerator(cfg, generatorFunc(func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
				t.Fatal("SDK called for invalid input")
				return nil, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Generate(context.Background(), tc.request); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestClientRejectsInvalidMediaBeforeCallingSDK(t *testing.T) {
	validPNG := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	for _, tc := range []struct {
		name  string
		media *MediaInput
		limit int
		want  error
	}{
		{name: "empty", media: &MediaInput{MIMEType: "image/png"}, limit: 100, want: ErrEmptyMedia},
		{name: "too large", media: &MediaInput{Data: validPNG, MIMEType: "image/png"}, limit: 7, want: ErrMediaTooLarge},
		{name: "unsupported", media: &MediaInput{Data: []byte("GIF89a"), MIMEType: "image/gif"}, limit: 100, want: ErrUnsupportedMedia},
		{name: "signature mismatch", media: &MediaInput{Data: validPNG, MIMEType: "image/jpeg"}, limit: 100, want: ErrMediaTypeMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.MaxMediaBytes = tc.limit
			client, err := NewClientWithGenerator(cfg, generatorFunc(func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
				t.Fatal("SDK called for invalid media")
				return nil, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Generate(t.Context(), GenerateRequest{Media: tc.media}); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestClientTimeoutAndParentDeadline(t *testing.T) {
	for _, shorterParent := range []bool{false, true} {
		cfg := testConfig()
		cfg.Timeout = 20 * time.Millisecond
		parent := context.Background()
		cancel := func() {}
		if shorterParent {
			cfg.Timeout = time.Second
			parent, cancel = context.WithTimeout(parent, 20*time.Millisecond)
		}
		client, err := NewClientWithGenerator(cfg, generatorFunc(func(ctx context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}))
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		_, err = client.Generate(parent, GenerateRequest{Text: "test"})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline error, got %v", err)
		}
	}
}

func TestClientCancelledContextDoesNotCallSDK(t *testing.T) {
	client, err := NewClientWithGenerator(testConfig(), generatorFunc(func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		t.Fatal("SDK called with cancelled context")
		return nil, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Generate(ctx, GenerateRequest{Text: "test"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestClientPreservesSDKErrorWithoutRetry(t *testing.T) {
	want := errors.New("simulated provider failure")
	calls := 0
	client, err := NewClientWithGenerator(testConfig(), generatorFunc(func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		calls++
		return nil, want
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Generate(context.Background(), GenerateRequest{Text: "test"}); !errors.Is(err, want) || calls != 1 {
		t.Fatalf("unexpected error or retry count: %v, calls=%d", err, calls)
	}
}

func TestNewClientRequiresExplicitKey(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "unrelated-test-key")
	t.Setenv("GEMINI_API_KEY", "unrelated-test-key")
	if _, err := NewClient(context.Background(), testConfig()); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("expected explicit key requirement, got %v", err)
	}
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	cfg := testConfig()
	cfg.APIKey = "fake-key-not-sent"
	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if client.cfg.APIKey != "" {
		t.Fatal("wrapper retained the credential")
	}
}

func TestNewClientRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewClientWithGenerator(testConfig(), nil); err == nil {
		t.Fatal("nil generator accepted")
	}
	cfg := testConfig()
	cfg.Timeout = 0
	if _, err := NewClient(context.Background(), cfg); err == nil {
		t.Fatal("invalid timeout accepted")
	}
}
