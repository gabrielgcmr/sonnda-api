// internal/features/documentprocessing/http/lab_extraction.go
package http

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

const standaloneLabExtractionMaxFileSize = 10 * 1024 * 1024

type pdfExtractor interface {
	ExtractPDF(ctx context.Context, currentAccount *accountdomain.Account, path, filename string) (*extraction.Result, error)
}

type StandaloneLabExtractionHandler struct {
	extractor pdfExtractor
}

type standaloneLabExtractionForm struct {
	File huma.FormFile `form:"file" required:"true"`
}

type standaloneLabExtractionInput struct {
	RawBody huma.MultipartFormFiles[standaloneLabExtractionForm]
}

type standaloneLabExtractionResponse = extraction.Result

type standaloneLabExtractionOutput struct {
	Body standaloneLabExtractionResponse
}

func NewStandaloneLabExtraction(extractor pdfExtractor) *StandaloneLabExtractionHandler {
	return &StandaloneLabExtractionHandler{extractor: extractor}
}

func (h *StandaloneLabExtractionHandler) RegisterHumaRoutes(registered huma.API, security []map[string][]string) {
	huma.Register(registered, huma.Operation{
		OperationID:  "extractStandaloneLabReport",
		Method:       http.MethodPost,
		Path:         "/lab-extractions",
		Summary:      "Extrair dados laboratoriais de forma independente sem persistir o documento",
		MaxBodyBytes: 11 * 1024 * 1024,
		Tags:         []string{"Lab extraction"},
		Errors:       []int{http.StatusUnauthorized, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity},
		Security:     security,
	}, h.extract)
}

func (h *StandaloneLabExtractionHandler) extract(ctx context.Context, input *standaloneLabExtractionInput) (*standaloneLabExtractionOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}

	files := input.RawBody.Form.File["file"]
	if len(files) != 1 {
		return nil, huma.Error422UnprocessableEntity("arquivo PDF e obrigatorio")
	}
	path, err := writeTemporaryPDF(files[0])
	if err != nil {
		return nil, humaerror.From(err)
	}
	defer os.Remove(path)

	result, err := h.extractor.ExtractPDF(ctx, currentAccount, path, files[0].Filename)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &standaloneLabExtractionOutput{Body: *result}, nil
}

func writeTemporaryPDF(header *multipart.FileHeader) (string, error) {
	if header == nil {
		return "", apperr.Validation("arquivo PDF e obrigatorio", apperr.Violation{Field: "file", Reason: "required"})
	}
	if header.Size <= 0 {
		return "", apperr.Validation("arquivo vazio", apperr.Violation{Field: "file", Reason: "empty"})
	}
	if header.Size > standaloneLabExtractionMaxFileSize {
		return "", &apperr.AppError{Kind: apperr.UPLOAD_SIZE_EXCEEDED, Message: "o PDF deve ter no maximo 10 MB"}
	}

	source, err := header.Open()
	if err != nil {
		return "", apperr.Internal("falha ao abrir arquivo", err)
	}
	defer source.Close()
	signature := make([]byte, 5)
	if _, err := io.ReadFull(source, signature); err != nil || !bytes.Equal(signature, []byte("%PDF-")) {
		return "", apperr.Validation("Envie um arquivo PDF válido.")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", apperr.Internal("falha ao ler arquivo", err)
	}
	target, err := os.CreateTemp("", "sonnda-lab-extraction-*.pdf")
	if err != nil {
		return "", apperr.Internal("falha ao preparar arquivo temporario", err)
	}
	path := target.Name()
	count, err := io.Copy(target, io.LimitReader(source, standaloneLabExtractionMaxFileSize+1))
	if err != nil {
		target.Close()
		os.Remove(path)
		return "", apperr.Internal("falha ao preparar arquivo temporario", err)
	}
	if count > standaloneLabExtractionMaxFileSize {
		_ = target.Close()
		_ = os.Remove(path)
		return "", &apperr.AppError{Kind: apperr.UPLOAD_SIZE_EXCEEDED, Message: "O PDF deve ter no máximo 10 MB."}
	}
	if err := target.Close(); err != nil {
		os.Remove(path)
		return "", apperr.Internal("falha ao preparar arquivo temporario", err)
	}
	return path, nil
}
