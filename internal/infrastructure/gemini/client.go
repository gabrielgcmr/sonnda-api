// internal/infrastructure/gemini/client.go
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gabrielgcmr/sonnda/internal/config"
	"google.golang.org/genai"
)

var (
	ErrMissingAPIKey     = errors.New("GEMINI_API_KEY is required")
	ErrEmptyInput        = errors.New("gemini input is required")
	ErrEmptyMedia        = errors.New("gemini media is empty")
	ErrInputTooLarge     = errors.New("gemini input exceeds configured byte limit")
	ErrMediaTooLarge     = errors.New("gemini media exceeds configured byte limit")
	ErrUnsupportedMedia  = errors.New("gemini media type is not supported")
	ErrMediaTypeMismatch = errors.New("gemini media content does not match its type")
)

// ContentGenerator permite simular o SDK sem chamadas externas.
type ContentGenerator interface {
	GenerateContent(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

var _ ContentGenerator = (*genai.Models)(nil)

type GenerateRequest struct {
	Text              string
	Media             *MediaInput
	SystemInstruction string
	JSONSchema        map[string]any
}

type MediaInput struct {
	Data     []byte
	MIMEType string
}

type Client struct {
	generator ContentGenerator
	cfg       config.GeminiConfig
}

func NewClient(ctx context.Context, cfg config.GeminiConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrMissingAPIKey
	}
	attempts := int32(1)
	sdk, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  strings.TrimSpace(cfg.APIKey),
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			Timeout:      &cfg.Timeout,
			RetryOptions: &genai.HTTPRetryOptions{Attempts: &attempts},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}
	return NewClientWithGenerator(cfg, sdk.Models)
}

func NewClientWithGenerator(cfg config.GeminiConfig, generator ContentGenerator) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if generator == nil {
		return nil, errors.New("gemini content generator is required")
	}
	// A credencial fica somente no SDK, nao na configuracao retida pelo wrapper.
	cfg.APIKey = ""
	cfg.Model = strings.TrimSpace(cfg.Model)
	return &Client{generator: generator, cfg: cfg}, nil
}

// Generate devolve a resposta crua; interpretacao laboratorial fica no adaptador.
func (c *Client) Generate(ctx context.Context, request GenerateRequest) (*genai.GenerateContentResponse, error) {
	if strings.TrimSpace(request.Text) == "" && request.Media == nil {
		return nil, ErrEmptyInput
	}
	remaining := c.cfg.MaxInputBytes
	for _, part := range []string{request.Text, request.SystemInstruction} {
		if len(part) > remaining {
			return nil, ErrInputTooLarge
		}
		remaining -= len(part)
	}
	if request.JSONSchema != nil {
		schema, err := json.Marshal(request.JSONSchema)
		if err != nil {
			return nil, errors.New("gemini response schema is not valid JSON")
		}
		if len(schema) > remaining {
			return nil, ErrInputTooLarge
		}
	}
	mediaMIMEType := ""
	if request.Media != nil {
		if len(request.Media.Data) == 0 {
			return nil, ErrEmptyMedia
		}
		if len(request.Media.Data) > c.cfg.MaxMediaBytes {
			return nil, ErrMediaTooLarge
		}
		mediaMIMEType = strings.ToLower(strings.TrimSpace(request.Media.MIMEType))
		if !supportedMediaType(mediaMIMEType) {
			return nil, ErrUnsupportedMedia
		}
		if !mediaMatchesType(request.Media.Data, mediaMIMEType) {
			return nil, ErrMediaTypeMismatch
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	if err := callCtx.Err(); err != nil {
		return nil, err
	}
	generation := &genai.GenerateContentConfig{
		CandidateCount:  1,
		MaxOutputTokens: c.cfg.MaxOutputTokens,
	}
	if request.SystemInstruction != "" {
		generation.SystemInstruction = genai.NewContentFromText(request.SystemInstruction, genai.RoleUser)
	}
	if request.JSONSchema != nil {
		generation.ResponseMIMEType = "application/json"
		generation.ResponseJsonSchema = request.JSONSchema
	}
	parts := make([]*genai.Part, 0, 2)
	if strings.TrimSpace(request.Text) != "" {
		parts = append(parts, genai.NewPartFromText(request.Text))
	}
	if request.Media != nil {
		parts = append(parts, genai.NewPartFromBytes(request.Media.Data, mediaMIMEType))
	}
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}
	response, err := c.generator.GenerateContent(callCtx, c.cfg.Model, contents, generation)
	if err != nil {
		return nil, fmt.Errorf("gemini generate content: %w", err)
	}
	return response, nil
}

func supportedMediaType(mimeType string) bool {
	switch mimeType {
	case "application/pdf", "image/jpeg", "image/png":
		return true
	default:
		return false
	}
}

func mediaMatchesType(data []byte, mimeType string) bool {
	switch mimeType {
	case "application/pdf":
		return len(data) >= 5 && string(data[:5]) == "%PDF-"
	case "image/jpeg":
		return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
	case "image/png":
		return len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
	default:
		return false
	}
}
