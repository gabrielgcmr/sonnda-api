// internal/features/documentprocessing/textreading/extractor.go
package textreading

import "context"

type ExtractInput struct {
	LocalPath        string
	DocumentURI      string
	MimeType         string
	OriginalFilename string
}

type ExtractOutput struct {
	// Text preserva a saida bruta do extrator para auditoria.
	Text string
	// NormalizedText contem apenas correcoes seguras para uso na aplicacao.
	NormalizedText string
	Method         string
}

type Extractor interface {
	Extract(ctx context.Context, input ExtractInput) (*ExtractOutput, error)
}
