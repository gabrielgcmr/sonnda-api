// internal/features/documentprocessing/http/lab_extraction.go
package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/api/humaspec"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	labreport "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

const standaloneLabExtractionMaxFileSize int64 = 10 * 1024 * 1024

type standaloneLabDocumentExtractor interface {
	Authorize(currentAccount *accountdomain.Account) error
	ExtractDocument(ctx context.Context, currentAccount *accountdomain.Account, path, filename, mimeType string) (*labreport.Result, error)
}

// CapturedFile is an open capture body; the handler closes Body.
type CapturedFile struct {
	Body      io.ReadCloser
	Filename  string
	MIMEType  string
	SizeBytes int64
}

// CapturedFileSource opens a capture owned by the account, if it is still available.
type CapturedFileSource interface {
	OpenCapturedFile(ctx context.Context, accountID, captureID uuid.UUID) (*CapturedFile, error)
}

type StandaloneLabExtractionHandler struct {
	extractor standaloneLabDocumentExtractor
	captures  CapturedFileSource
}

type standaloneLabExtractionForm struct {
	File huma.FormFile `form:"file" required:"true"`
}

type standaloneLabExtractionInput struct {
	RawBody huma.MultipartFormFiles[standaloneLabExtractionForm]
}

type standaloneLabExtractionResponse = labreport.Result

type standaloneLabExtractionOutput struct {
	Body standaloneLabExtractionResponse
}

type captureLabExtractionRequest struct {
	CaptureID uuid.UUID `json:"capture_id" format:"uuid" doc:"Captura PDF, JPEG ou PNG disponível da conta autenticada"`
}

type captureLabExtractionInput struct {
	Body captureLabExtractionRequest
}

func NewStandaloneLabExtraction(extractor standaloneLabDocumentExtractor, captures CapturedFileSource) *StandaloneLabExtractionHandler {
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
		Summary:      "Extrair dados laboratoriais de PDF, JPEG ou PNG sem persistir o documento",
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
		return nil, humaerror.From(apperr.Validation("arquivo é obrigatório", apperr.Violation{Field: "file", Reason: "required"}))
	}
	document, err := writeTemporaryDocument(files[0], standaloneLabExtractionMaxFileSize)
	if err != nil {
		return nil, humaerror.From(err)
	}
	defer os.Remove(document.path)

	result, err := h.extractor.ExtractDocument(ctx, currentAccount, document.path, files[0].Filename, document.mimeType)
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
		Summary:     "Extrair dados laboratoriais de uma captura PDF, JPEG ou PNG",
		Description: "Lê no servidor uma captura disponível da conta. A captura permanece disponível até ser excluída ou expirar.",
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
		return nil, humaerror.From(apperr.Internal("capturas indisponíveis", errors.New("captured file source not configured")))
	}
	if err := h.extractor.Authorize(currentAccount); err != nil {
		return nil, humaerror.From(err)
	}

	file, err := h.captures.OpenCapturedFile(ctx, currentAccount.ID, input.Body.CaptureID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	if file == nil || file.Body == nil {
		return nil, humaerror.From(apperr.Internal("captura indisponível", errors.New("capture source returned no body")))
	}
	document, copyErr := copyTemporaryDocument(file.Body, file.MIMEType, standaloneLabExtractionMaxFileSize)
	closeErr := file.Body.Close()
	if copyErr != nil {
		return nil, humaerror.From(copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(document.path)
		return nil, humaerror.From(apperr.Internal("falha ao fechar captura", closeErr))
	}
	defer os.Remove(document.path)

	result, err := h.extractor.ExtractDocument(ctx, currentAccount, document.path, file.Filename, document.mimeType)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &standaloneLabExtractionOutput{Body: *result}, nil
}
