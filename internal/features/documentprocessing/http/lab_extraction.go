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
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/api/humaspec"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

const standaloneLabExtractionMaxFileSize int64 = 10 * 1024 * 1024

type documentExtractor interface {
	Authorize(currentAccount *accountdomain.Account) error
	ExtractDocument(ctx context.Context, currentAccount *accountdomain.Account, path, filename, mimeType string) (*extraction.Result, error)
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
	extractor documentExtractor
	captures  CapturedFileSource
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
	CaptureID uuid.UUID `json:"capture_id" format:"uuid" doc:"Captura PDF, JPEG ou PNG disponível da conta autenticada"`
}

type captureLabExtractionInput struct {
	Body captureLabExtractionRequest
}

type temporaryDocument struct {
	path     string
	mimeType string
}

func NewStandaloneLabExtraction(extractor documentExtractor, captures CapturedFileSource) *StandaloneLabExtractionHandler {
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

func writeTemporaryDocument(header *multipart.FileHeader, maxFileSize int64) (*temporaryDocument, error) {
	if header == nil {
		return nil, apperr.Validation("arquivo é obrigatório", apperr.Violation{Field: "file", Reason: "required"})
	}
	if header.Size <= 0 {
		return nil, apperr.Validation("arquivo vazio", apperr.Violation{Field: "file", Reason: "empty"})
	}
	if header.Size > maxFileSize {
		return nil, documentSizeExceededError(maxFileSize)
	}

	source, err := header.Open()
	if err != nil {
		return nil, apperr.Internal("falha ao abrir arquivo", err)
	}
	defer source.Close()
	return copyTemporaryDocument(source, "", maxFileSize)
}

// writeTemporaryPDF preserves the PDF-only contract used by persisted patient documents.
func writeTemporaryPDF(header *multipart.FileHeader, maxFileSize int64) (string, error) {
	document, err := writeTemporaryDocument(header, maxFileSize)
	if err != nil {
		return "", err
	}
	if document.mimeType != "application/pdf" {
		_ = os.Remove(document.path)
		return "", apperr.Validation("Envie um arquivo PDF válido.")
	}
	return document.path, nil
}

func copyTemporaryDocument(source io.Reader, declaredMIME string, maxFileSize int64) (*temporaryDocument, error) {
	prefix := make([]byte, 8)
	count, err := io.ReadFull(source, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, apperr.Internal("falha ao ler arquivo", err)
	}
	prefix = prefix[:count]
	mimeType, extension := detectDocumentType(prefix)
	if mimeType == "" || (declaredMIME != "" && normalizeDocumentMIME(declaredMIME) != mimeType) {
		return nil, &apperr.AppError{Kind: apperr.UNSUPPORTED_MEDIA_TYPE, Message: "Envie um arquivo PDF, JPEG ou PNG válido."}
	}

	target, err := os.CreateTemp("", "sonnda-lab-extraction-*"+extension)
	if err != nil {
		return nil, apperr.Internal("falha ao preparar arquivo temporário", err)
	}
	path := target.Name()
	combined := io.MultiReader(bytes.NewReader(prefix), source)
	written, copyErr := io.Copy(target, io.LimitReader(combined, maxFileSize+1))
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return nil, apperr.Internal("falha ao preparar arquivo temporário", errors.Join(copyErr, closeErr))
	}
	if written > maxFileSize {
		_ = os.Remove(path)
		return nil, documentSizeExceededError(maxFileSize)
	}
	return &temporaryDocument{path: path, mimeType: mimeType}, nil
}

func detectDocumentType(prefix []byte) (string, string) {
	switch {
	case len(prefix) >= 5 && bytes.Equal(prefix[:5], []byte("%PDF-")):
		return "application/pdf", ".pdf"
	case len(prefix) >= 3 && bytes.Equal(prefix[:3], []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg", ".jpg"
	case len(prefix) >= 8 && bytes.Equal(prefix[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png", ".png"
	default:
		return "", ""
	}
}

func normalizeDocumentMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "application/pdf", "application/x-pdf", "image/pdf":
		return "application/pdf"
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	default:
		return ""
	}
}

func documentSizeExceededError(maxFileSize int64) error {
	return &apperr.AppError{
		Kind:    apperr.UPLOAD_SIZE_EXCEEDED,
		Message: fmt.Sprintf("o arquivo deve ter no máximo %d MiB", maxFileSize/(1024*1024)),
	}
}
