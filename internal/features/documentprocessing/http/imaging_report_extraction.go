// internal/features/documentprocessing/http/imaging_report_extraction.go
package http

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/api/humaspec"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type standaloneImagingExtractor interface {
	Authorize(*accountdomain.Account) error
	ExtractDocument(context.Context, *accountdomain.Account, string, string, string) (*imagingreport.ImagingReportResult, error)
}

type StandaloneImagingExtractionHandler struct {
	extractor standaloneImagingExtractor
	captures  CapturedFileSource
}

type imagingExtractionForm struct {
	File huma.FormFile `form:"file" required:"true"`
}
type imagingExtractionInput struct {
	RawBody huma.MultipartFormFiles[imagingExtractionForm]
}
type imagingCaptureRequest struct {
	CaptureID uuid.UUID `json:"capture_id" format:"uuid" doc:"Captura PDF, JPEG ou PNG da conta autenticada"`
}
type imagingCaptureInput struct{ Body imagingCaptureRequest }
type imagingExtractionOutput struct {
	Body imagingreport.ImagingReportResult
}

func NewStandaloneImagingExtraction(extractor standaloneImagingExtractor, captures CapturedFileSource) *StandaloneImagingExtractionHandler {
	return &StandaloneImagingExtractionHandler{extractor: extractor, captures: captures}
}

func (h *StandaloneImagingExtractionHandler) RegisterHumaRoutes(registered huma.API, security []map[string][]string) {
	registerImagingExtractionRoutes(registered, security, h.extract, h.extractFromCapture)
}

func RegisterStandaloneImagingExtractionOpenAPIRoutes(registered huma.API, security []map[string][]string) {
	registerImagingExtractionRoutes(registered, security,
		humaspec.NotImplemented[imagingExtractionInput, imagingExtractionOutput],
		humaspec.NotImplemented[imagingCaptureInput, imagingExtractionOutput])
}

func registerImagingExtractionRoutes(
	registered huma.API,
	security []map[string][]string,
	upload func(context.Context, *imagingExtractionInput) (*imagingExtractionOutput, error),
	capture func(context.Context, *imagingCaptureInput) (*imagingExtractionOutput, error),
) {
	huma.Register(registered, huma.Operation{
		OperationID:  "extractImagingReport",
		Method:       http.MethodPost,
		Path:         "/imaging-report-extractions",
		Summary:      "Extrair dados de um laudo escrito de exame de imagem",
		Description:  "Aceita um PDF, JPEG ou PNG com um único laudo escrito. Não interpreta imagens radiológicas isoladas.",
		MaxBodyBytes: 11 * 1024 * 1024,
		Tags:         []string{"Imaging report extraction"},
		Errors:       []int{http.StatusUnauthorized, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity},
		Security:     security,
	}, upload)
	huma.Register(registered, huma.Operation{
		OperationID: "extractImagingReportFromCapture",
		Method:      http.MethodPost,
		Path:        "/imaging-report-extractions/from-capture",
		Summary:     "Extrair dados de um laudo escrito recebido em Capturas",
		Description: "Lê no servidor uma captura disponível da conta autenticada.",
		Tags:        []string{"Imaging report extraction"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity},
		Security:    security,
	}, capture)
}

func (h *StandaloneImagingExtractionHandler) extract(ctx context.Context, input *imagingExtractionInput) (*imagingExtractionOutput, error) {
	account, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}
	if err := h.extractor.Authorize(account); err != nil {
		return nil, humaerror.From(err)
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
	result, err := h.extractor.ExtractDocument(ctx, account, document.path, files[0].Filename, document.mimeType)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &imagingExtractionOutput{Body: *result}, nil
}

func (h *StandaloneImagingExtractionHandler) extractFromCapture(ctx context.Context, input *imagingCaptureInput) (*imagingExtractionOutput, error) {
	account, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}
	if h.captures == nil {
		return nil, humaerror.From(apperr.Internal("capturas indisponíveis", errors.New("captured file source not configured")))
	}
	if err := h.extractor.Authorize(account); err != nil {
		return nil, humaerror.From(err)
	}
	file, err := h.captures.OpenCapturedFile(ctx, account.ID, input.Body.CaptureID)
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
	result, err := h.extractor.ExtractDocument(ctx, account, document.path, file.Filename, document.mimeType)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &imagingExtractionOutput{Body: *result}, nil
}
