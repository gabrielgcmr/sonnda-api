// internal/features/documentprocessing/http/imaging_report_pages.go
package http

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type imagingPagesForm struct {
	Files []huma.FormFile `form:"files" required:"true"`
}
type imagingPagesInput struct {
	RawBody huma.MultipartFormFiles[imagingPagesForm]
}
type imagingCapturesRequest struct {
	CaptureIDs []uuid.UUID `json:"capture_ids" doc:"Capturas em ordem de página, da conta autenticada"`
}
type imagingCapturesInput struct{ Body imagingCapturesRequest }

func registerImagingPageRoutes(api huma.API, security []map[string][]string,
	upload func(context.Context, *imagingPagesInput) (*imagingExtractionOutput, error),
	captures func(context.Context, *imagingCapturesInput) (*imagingExtractionOutput, error),
) {
	huma.Register(api, huma.Operation{
		OperationID: "extractImagingReportPages", Method: http.MethodPost,
		Path: "/imaging-report-extractions/pages", Summary: "Extrair um laudo escrito de 2 a 5 imagens em ordem de página",
		MaxBodyBytes: 11 * 1024 * 1024, Tags: []string{"Imaging report extraction"}, Security: security,
		Errors: []int{http.StatusUnauthorized, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity},
	}, upload)
	huma.Register(api, huma.Operation{
		OperationID: "extractImagingReportFromCaptures", Method: http.MethodPost,
		Path: "/imaging-report-extractions/from-captures", Summary: "Extrair um laudo escrito de 2 a 5 capturas em ordem de página",
		Tags: []string{"Imaging report extraction"}, Security: security,
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity},
	}, captures)
}

func validatePageCount(count int) error {
	if count < 2 || count > imagingreport.MaxImagePages {
		return apperr.Validation("Selecione de 2 a 5 imagens do mesmo laudo.", apperr.Violation{Field: "files", Reason: "invalid_page_count"})
	}
	return nil
}

func addImagingPage(pages *[]imagingreport.DocumentPage, document *temporaryDocument, name string, total *int64) error {
	if document.mimeType != "image/jpeg" && document.mimeType != "image/png" {
		return &apperr.AppError{Kind: apperr.UNSUPPORTED_MEDIA_TYPE, Message: "Envie apenas imagens JPEG ou PNG para um laudo com várias páginas."}
	}
	info, err := os.Stat(document.path)
	if err != nil {
		return apperr.Internal("Não foi possível ler a imagem temporária.", err)
	}
	*total += info.Size()
	if *total > standaloneLabExtractionMaxFileSize {
		return documentSizeExceededError(standaloneLabExtractionMaxFileSize)
	}
	*pages = append(*pages, imagingreport.DocumentPage{Path: document.path, Filename: name, MIMEType: document.mimeType})
	return nil
}

func (h *StandaloneImagingExtractionHandler) extractPages(ctx context.Context, input *imagingPagesInput) (*imagingExtractionOutput, error) {
	account, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}
	if err := h.extractor.Authorize(account); err != nil {
		return nil, humaerror.From(err)
	}
	files := input.RawBody.Form.File["files"]
	if err := validatePageCount(len(files)); err != nil {
		return nil, humaerror.From(err)
	}
	pages := make([]imagingreport.DocumentPage, 0, len(files))
	var paths []string
	defer func() {
		for _, path := range paths {
			_ = os.Remove(path)
		}
	}()
	var total int64
	for _, file := range files {
		document, err := writeTemporaryDocument(file, standaloneLabExtractionMaxFileSize)
		if err != nil {
			return nil, humaerror.From(err)
		}
		paths = append(paths, document.path)
		if err := addImagingPage(&pages, document, file.Filename, &total); err != nil {
			return nil, humaerror.From(err)
		}
	}
	result, err := h.extractor.ExtractImagePages(ctx, account, pages)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &imagingExtractionOutput{Body: *result}, nil
}

func (h *StandaloneImagingExtractionHandler) extractFromCaptures(ctx context.Context, input *imagingCapturesInput) (*imagingExtractionOutput, error) {
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
	if err := validatePageCount(len(input.Body.CaptureIDs)); err != nil {
		return nil, humaerror.From(err)
	}
	pages := make([]imagingreport.DocumentPage, 0, len(input.Body.CaptureIDs))
	var paths []string
	defer func() {
		for _, path := range paths {
			_ = os.Remove(path)
		}
	}()
	seen := make(map[uuid.UUID]bool, len(input.Body.CaptureIDs))
	var total int64
	for _, id := range input.Body.CaptureIDs {
		if id == uuid.Nil || seen[id] {
			return nil, humaerror.From(apperr.Validation("Cada página deve ser uma captura distinta.", apperr.Violation{Field: "capture_ids", Reason: "duplicate_or_empty"}))
		}
		seen[id] = true
		file, err := h.captures.OpenCapturedFile(ctx, account.ID, id)
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
		paths = append(paths, document.path)
		if closeErr != nil {
			return nil, humaerror.From(apperr.Internal("falha ao fechar captura", closeErr))
		}
		if err := addImagingPage(&pages, document, file.Filename, &total); err != nil {
			return nil, humaerror.From(err)
		}
	}
	result, err := h.extractor.ExtractImagePages(ctx, account, pages)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &imagingExtractionOutput{Body: *result}, nil
}
