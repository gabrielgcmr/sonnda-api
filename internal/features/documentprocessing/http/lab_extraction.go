// internal/features/documentprocessing/http/lab_extraction.go
package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/api/humaspec"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

const standaloneLabExtractionMaxFileSize = 10 * 1024 * 1024

type pdfExtractor interface {
	Authorize(currentAccount *accountdomain.Account) error
	ExtractPDF(ctx context.Context, currentAccount *accountdomain.Account, path, filename string) (*extraction.Result, error)
}

// CapturedPDF is an open capture body; the handler closes Body.
type CapturedPDF struct {
	Body     io.ReadCloser
	Filename string
}

// CapturedPDFSource opens a PDF capture owned by the account, if it is still available.
type CapturedPDFSource interface {
	OpenCapturedPDF(ctx context.Context, accountID, captureID uuid.UUID) (*CapturedPDF, error)
}

type StandaloneLabExtractionHandler struct {
	extractor pdfExtractor
	captures  CapturedPDFSource
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

type captureLabExtractionRequest struct {
	CaptureID uuid.UUID `json:"capture_id" format:"uuid" doc:"Captura PDF disponível da conta autenticada"`
}

type captureLabExtractionInput struct {
	Body captureLabExtractionRequest
}

func NewStandaloneLabExtraction(extractor pdfExtractor, captures CapturedPDFSource) *StandaloneLabExtractionHandler {
	return &StandaloneLabExtractionHandler{extractor: extractor, captures: captures}
}

func (h *StandaloneLabExtractionHandler) RegisterHumaRoutes(registered huma.API, security []map[string][]string) {
	registerStandaloneLabExtractionRoute(registered, security, h.extract)
	registerCaptureLabExtractionRoute(registered, security, h.extractFromCapture)
}

// RegisterStandaloneLabExtractionOpenAPIRoutes registers extraction metadata without runtime dependencies.
func RegisterStandaloneLabExtractionOpenAPIRoutes(registered huma.API, security []map[string][]string) {
	registerStandaloneLabExtractionRoute(registered, security, humaspec.NotImplemented[standaloneLabExtractionInput, standaloneLabExtractionOutput])
	registerCaptureLabExtractionRoute(registered, security, humaspec.NotImplemented[captureLabExtractionInput, standaloneLabExtractionOutput])
}

func registerStandaloneLabExtractionRoute(
	registered huma.API,
	security []map[string][]string,
	endpoint func(context.Context, *standaloneLabExtractionInput) (*standaloneLabExtractionOutput, error),
) {
	huma.Register(registered, huma.Operation{
		OperationID:  "extractStandaloneLabReport",
		Method:       http.MethodPost,
		Path:         "/lab-extractions",
		Summary:      "Extrair dados laboratoriais de forma independente sem persistir o documento",
		MaxBodyBytes: 11 * 1024 * 1024,
		Tags:         []string{"Lab extraction"},
		Errors:       []int{http.StatusUnauthorized, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity},
		Security:     security,
	}, endpoint)
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
	path, err := writeTemporaryPDF(files[0], standaloneLabExtractionMaxFileSize)
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

func registerCaptureLabExtractionRoute(
	registered huma.API,
	security []map[string][]string,
	endpoint func(context.Context, *captureLabExtractionInput) (*standaloneLabExtractionOutput, error),
) {
	huma.Register(registered, huma.Operation{
		OperationID: "extractStandaloneLabReportFromCapture",
		Method:      http.MethodPost,
		Path:        "/lab-extractions/from-capture",
		Summary:     "Extrair dados laboratoriais de uma captura PDF sem associá-la a paciente",
		Description: "Lê no servidor uma captura PDF disponível da conta. A captura permanece disponível até ser excluída ou expirar.",
		Tags:        []string{"Lab extraction"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity},
		Security:    security,
	}, endpoint)
}

func (h *StandaloneLabExtractionHandler) extractFromCapture(ctx context.Context, input *captureLabExtractionInput) (*standaloneLabExtractionOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}
	if h.captures == nil {
		return nil, humaerror.From(apperr.Internal("capturas indisponíveis", errors.New("captured PDF source not configured")))
	}
	if err := h.extractor.Authorize(currentAccount); err != nil {
		return nil, humaerror.From(err)
	}

	file, err := h.captures.OpenCapturedPDF(ctx, currentAccount.ID, input.Body.CaptureID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	path, err := copyTemporaryPDF(file.Body, standaloneLabExtractionMaxFileSize)
	file.Body.Close()
	if err != nil {
		return nil, humaerror.From(err)
	}
	defer os.Remove(path)

	result, err := h.extractor.ExtractPDF(ctx, currentAccount, path, file.Filename)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &standaloneLabExtractionOutput{Body: *result}, nil
}

func writeTemporaryPDF(header *multipart.FileHeader, maxFileSize int64) (string, error) {
	if header == nil {
		return "", apperr.Validation("arquivo PDF e obrigatorio", apperr.Violation{Field: "file", Reason: "required"})
	}
	if header.Size <= 0 {
		return "", apperr.Validation("arquivo vazio", apperr.Violation{Field: "file", Reason: "empty"})
	}
	if header.Size > maxFileSize {
		return "", pdfSizeExceededError(maxFileSize)
	}

	source, err := header.Open()
	if err != nil {
		return "", apperr.Internal("falha ao abrir arquivo", err)
	}
	defer source.Close()
	return copyTemporaryPDF(source, maxFileSize)
}

func copyTemporaryPDF(source io.Reader, maxFileSize int64) (string, error) {
	signature := make([]byte, 5)
	if _, err := io.ReadFull(source, signature); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", apperr.Internal("falha ao ler arquivo", err)
	} else if err != nil || !bytes.Equal(signature, []byte("%PDF-")) {
		return "", apperr.Validation("Envie um arquivo PDF válido.")
	}
	source = io.MultiReader(bytes.NewReader(signature), source)
	target, err := os.CreateTemp("", "sonnda-lab-extraction-*.pdf")
	if err != nil {
		return "", apperr.Internal("falha ao preparar arquivo temporario", err)
	}
	path := target.Name()
	count, err := io.Copy(target, io.LimitReader(source, maxFileSize+1))
	if err != nil {
		target.Close()
		os.Remove(path)
		return "", apperr.Internal("falha ao preparar arquivo temporario", err)
	}
	if count > maxFileSize {
		_ = target.Close()
		_ = os.Remove(path)
		return "", pdfSizeExceededError(maxFileSize)
	}
	if err := target.Close(); err != nil {
		os.Remove(path)
		return "", apperr.Internal("falha ao preparar arquivo temporario", err)
	}
	return path, nil
}

func pdfSizeExceededError(maxFileSize int64) error {
	return &apperr.AppError{
		Kind:    apperr.UPLOAD_SIZE_EXCEEDED,
		Message: fmt.Sprintf("o PDF deve ter no máximo %d MiB", maxFileSize/(1024*1024)),
	}
}
