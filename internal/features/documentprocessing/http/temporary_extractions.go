// internal/features/documentprocessing/http/temporary_extractions.go
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/api/humaspec"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type temporaryResultStore interface {
	Save(context.Context, uuid.UUID, string, any, []documentprocessing.TemporaryPage) (*documentprocessing.TemporaryResult, error)
	List(context.Context, uuid.UUID) ([]documentprocessing.TemporaryResult, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (*documentprocessing.TemporaryResult, error)
}

type TemporaryExtractionHandler struct {
	lab      standaloneLabDocumentExtractor
	imaging  standaloneImagingExtractor
	captures CapturedFileSource
	store    temporaryResultStore
}

func NewTemporaryExtractionHandler(lab standaloneLabDocumentExtractor, imaging standaloneImagingExtractor, captures CapturedFileSource, store temporaryResultStore) *TemporaryExtractionHandler {
	return &TemporaryExtractionHandler{lab: lab, imaging: imaging, captures: captures, store: store}
}

type temporaryUploadForm struct {
	Kind  string          `form:"kind" required:"true"`
	Files []huma.FormFile `form:"files" required:"true"`
}
type temporaryUploadInput struct {
	RawBody huma.MultipartFormFiles[temporaryUploadForm]
}
type temporaryCaptureRequest struct {
	Kind       string      `json:"kind" enum:"laboratory,imaging"`
	CaptureIDs []uuid.UUID `json:"capture_ids" minItems:"1" maxItems:"5"`
}
type temporaryCaptureInput struct{ Body temporaryCaptureRequest }
type temporaryPathInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}
type temporaryListInput struct{}

type temporaryResultResponse struct {
	ID            uuid.UUID                          `json:"id"`
	Kind          string                             `json:"kind"`
	CreatedAt     time.Time                          `json:"created_at"`
	ExpiresAt     time.Time                          `json:"expires_at"`
	Pages         []documentprocessing.TemporaryPage `json:"pages"`
	LabResult     *labreport.Result                  `json:"lab_result,omitempty"`
	ImagingResult *imagingreport.ImagingReportResult `json:"imaging_result,omitempty"`
}
type temporaryResultOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         temporaryResultResponse
}
type temporaryListResponse struct {
	Items []temporaryResultResponse `json:"items"`
}
type temporaryListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         temporaryListResponse
}

func (h *TemporaryExtractionHandler) RegisterHumaRoutes(api huma.API, security []map[string][]string) {
	registerTemporaryRoutes(api, security, h.fromUpload, h.fromCaptures, h.list, h.get)
}

func RegisterTemporaryExtractionOpenAPIRoutes(api huma.API, security []map[string][]string) {
	registerTemporaryRoutes(api, security,
		humaspec.NotImplemented[temporaryUploadInput, temporaryResultOutput],
		humaspec.NotImplemented[temporaryCaptureInput, temporaryResultOutput],
		humaspec.NotImplemented[temporaryListInput, temporaryListOutput],
		humaspec.NotImplemented[temporaryPathInput, temporaryResultOutput])
}

func registerTemporaryRoutes(api huma.API, security []map[string][]string,
	upload func(context.Context, *temporaryUploadInput) (*temporaryResultOutput, error),
	captures func(context.Context, *temporaryCaptureInput) (*temporaryResultOutput, error),
	list func(context.Context, *temporaryListInput) (*temporaryListOutput, error),
	get func(context.Context, *temporaryPathInput) (*temporaryResultOutput, error),
) {
	huma.Register(api, huma.Operation{OperationID: "createTemporaryExtraction", Method: http.MethodPost,
		Path: "/temporary-extractions", Summary: "Extrair e guardar temporariamente arquivos enviados",
		MaxBodyBytes: 11 * 1024 * 1024, Tags: []string{"Temporary extractions"}, Security: security}, upload)
	huma.Register(api, huma.Operation{OperationID: "createTemporaryExtractionFromCaptures", Method: http.MethodPost,
		Path: "/temporary-extractions/from-captures", Summary: "Extrair e guardar temporariamente capturas",
		Tags: []string{"Temporary extractions"}, Security: security}, captures)
	huma.Register(api, huma.Operation{OperationID: "listTemporaryExtractions", Method: http.MethodGet,
		Path: "/temporary-extractions", Summary: "Listar extrações temporárias disponíveis da conta",
		Tags: []string{"Temporary extractions"}, Security: security}, list)
	huma.Register(api, huma.Operation{OperationID: "getTemporaryExtraction", Method: http.MethodGet,
		Path: "/temporary-extractions/{id}", Summary: "Consultar uma extração temporária da conta",
		Tags: []string{"Temporary extractions"}, Security: security}, get)
}

func (h *TemporaryExtractionHandler) fromUpload(ctx context.Context, input *temporaryUploadInput) (*temporaryResultOutput, error) {
	account, err := temporaryAccount(ctx)
	if err != nil {
		return nil, humaerror.From(err)
	}
	kind := input.RawBody.Form.Value["kind"]
	if len(kind) != 1 {
		return nil, humaerror.From(apperr.Validation("tipo de extração obrigatório"))
	}
	files := input.RawBody.Form.File["files"]
	if err := validateTemporarySelection(kind[0], len(files)); err != nil {
		return nil, humaerror.From(err)
	}
	pages := make([]documentprocessing.TemporaryPage, 0, len(files))
	defer func() {
		for _, page := range pages {
			_ = os.Remove(page.LocalPath)
		}
	}()
	var total int64
	for _, file := range files {
		document, err := writeTemporaryDocument(file, standaloneLabExtractionMaxFileSize)
		if err != nil {
			return nil, humaerror.From(err)
		}
		info, err := os.Stat(document.path)
		if err != nil {
			_ = os.Remove(document.path)
			return nil, humaerror.From(apperr.Internal("falha ao ler arquivo", err))
		}
		page := documentprocessing.TemporaryPage{LocalPath: document.path, OriginalFilename: file.Filename,
			MIMEType: document.mimeType, SizeBytes: info.Size()}
		pages = append(pages, page)
		total += info.Size()
		if total > standaloneLabExtractionMaxFileSize || (len(files) > 1 && document.mimeType == "application/pdf") {
			return nil, humaerror.From(apperr.Validation("selecione até cinco imagens com 10 MiB no total"))
		}
	}
	return h.extractAndSave(ctx, account, kind[0], pages)
}

func (h *TemporaryExtractionHandler) fromCaptures(ctx context.Context, input *temporaryCaptureInput) (*temporaryResultOutput, error) {
	account, err := temporaryAccount(ctx)
	if err != nil {
		return nil, humaerror.From(err)
	}
	if err := validateTemporarySelection(input.Body.Kind, len(input.Body.CaptureIDs)); err != nil {
		return nil, humaerror.From(err)
	}
	if h.captures == nil {
		return nil, humaerror.From(apperr.Internal("capturas indisponíveis", errors.New("capture source missing")))
	}
	pages := make([]documentprocessing.TemporaryPage, 0, len(input.Body.CaptureIDs))
	defer func() {
		for _, page := range pages {
			_ = os.Remove(page.LocalPath)
		}
	}()
	seen := make(map[uuid.UUID]bool)
	var total int64
	for _, id := range input.Body.CaptureIDs {
		if id == uuid.Nil || seen[id] {
			return nil, humaerror.From(apperr.Validation("capturas repetidas ou inválidas"))
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
		if closeErr != nil {
			_ = os.Remove(document.path)
			return nil, humaerror.From(apperr.Internal("falha ao fechar captura", closeErr))
		}
		info, err := os.Stat(document.path)
		if err != nil {
			_ = os.Remove(document.path)
			return nil, humaerror.From(apperr.Internal("falha ao ler captura", err))
		}
		pages = append(pages, documentprocessing.TemporaryPage{LocalPath: document.path,
			CaptureID: &id, OriginalFilename: file.Filename, MIMEType: document.mimeType,
			SizeBytes: info.Size(), ExpiresAt: file.ExpiresAt})
		total += info.Size()
		if total > standaloneLabExtractionMaxFileSize || (len(input.Body.CaptureIDs) > 1 && document.mimeType == "application/pdf") {
			return nil, humaerror.From(apperr.Validation("selecione até cinco imagens com 10 MiB no total"))
		}
	}
	return h.extractAndSave(ctx, account, input.Body.Kind, pages)
}

func validateTemporarySelection(kind string, count int) error {
	if kind != "laboratory" && kind != "imaging" {
		return apperr.Validation("tipo de extração inválido")
	}
	if count < 1 || (kind == "laboratory" && count != 1) || count > 5 {
		return apperr.Validation("quantidade de arquivos inválida")
	}
	return nil
}

func (h *TemporaryExtractionHandler) extractAndSave(ctx context.Context, account *accountdomain.Account, kind string, pages []documentprocessing.TemporaryPage) (*temporaryResultOutput, error) {
	if h.store == nil {
		return nil, humaerror.From(apperr.Internal("extrações temporárias indisponíveis", errors.New("store missing")))
	}
	var result any
	if kind == "laboratory" {
		lab, err := h.lab.ExtractDocument(ctx, account, pages[0].LocalPath, pages[0].OriginalFilename, pages[0].MIMEType)
		if err != nil {
			return nil, humaerror.From(err)
		}
		result = lab
	} else if len(pages) == 1 {
		imaging, err := h.imaging.ExtractDocument(ctx, account, pages[0].LocalPath, pages[0].OriginalFilename, pages[0].MIMEType)
		if err != nil {
			return nil, humaerror.From(err)
		}
		result = imaging
	} else {
		input := make([]imagingreport.DocumentPage, len(pages))
		for i, page := range pages {
			input[i] = imagingreport.DocumentPage{Path: page.LocalPath, Filename: page.OriginalFilename, MIMEType: page.MIMEType}
		}
		imaging, err := h.imaging.ExtractImagePages(ctx, account, input)
		if err != nil {
			return nil, humaerror.From(err)
		}
		result = imaging
	}
	item, err := h.store.Save(ctx, account.ID, kind, result, pages)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return presentTemporaryResult(item)
}

func (h *TemporaryExtractionHandler) list(ctx context.Context, _ *temporaryListInput) (*temporaryListOutput, error) {
	if h.store == nil {
		return nil, humaerror.From(apperr.Internal("extrações temporárias indisponíveis", errors.New("store missing")))
	}
	account, err := temporaryAccount(ctx)
	if err != nil {
		return nil, humaerror.From(err)
	}
	items, err := h.store.List(ctx, account.ID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	response := make([]temporaryResultResponse, len(items))
	for i, item := range items {
		response[i] = temporaryResultResponse{ID: item.ID, Kind: item.Kind, CreatedAt: item.CreatedAt, ExpiresAt: item.ExpiresAt}
	}
	return &temporaryListOutput{CacheControl: "no-store", Body: temporaryListResponse{Items: response}}, nil
}

func (h *TemporaryExtractionHandler) get(ctx context.Context, input *temporaryPathInput) (*temporaryResultOutput, error) {
	if h.store == nil {
		return nil, humaerror.From(apperr.Internal("extrações temporárias indisponíveis", errors.New("store missing")))
	}
	account, err := temporaryAccount(ctx)
	if err != nil {
		return nil, humaerror.From(err)
	}
	item, err := h.store.Get(ctx, account.ID, input.ID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return presentTemporaryResult(item)
}

func presentTemporaryResult(item *documentprocessing.TemporaryResult) (*temporaryResultOutput, error) {
	response := temporaryResultResponse{ID: item.ID, Kind: item.Kind, CreatedAt: item.CreatedAt,
		ExpiresAt: item.ExpiresAt, Pages: item.Pages}
	if item.Kind == "laboratory" {
		var result labreport.Result
		if err := json.Unmarshal(item.Result, &result); err != nil {
			return nil, humaerror.From(apperr.Internal("resultado temporário inválido", err))
		}
		response.LabResult = &result
	} else {
		var result imagingreport.ImagingReportResult
		if err := json.Unmarshal(item.Result, &result); err != nil {
			return nil, humaerror.From(apperr.Internal("resultado temporário inválido", err))
		}
		response.ImagingResult = &result
	}
	return &temporaryResultOutput{CacheControl: "no-store", Body: response}, nil
}

func temporaryAccount(ctx context.Context) (*accountdomain.Account, error) {
	account, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok || account == nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}
	return account, nil
}
