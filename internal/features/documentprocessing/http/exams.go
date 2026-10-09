// internal/features/documentprocessing/http/exams.go
package http

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	documents "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	laboratory "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory"
	"github.com/google/uuid"
)

type draftService interface {
	Create(context.Context, documents.CreateDraftInput) (*documents.ExamDocumentOutput, error)
	Extraction(context.Context, uuid.UUID) (*extraction.Result, error)
	Delete(context.Context, uuid.UUID) error
}
type confirmationService interface {
	Confirm(context.Context, uuid.UUID, uuid.UUID) (*laboratory.LabReportOutput, error)
}
type ExamsHandler struct {
	svc           documents.Service
	drafts        draftService
	confirmer     confirmationService
	storage       documentprocessing.FileStorageService
	accessChecker patientaccess.Checker
}

const (
	examDocumentFileURLExpirationMinutes = 15
	examDocumentMaxFileSize              = 5 * 1024 * 1024
	examDocumentMaxBodySize              = examDocumentMaxFileSize + 1024*1024
)

type examDocumentFileResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type listExamDocumentsInput struct {
	PatientID uuid.UUID `path:"patientId" format:"uuid"`
	Limit     int       `query:"limit" default:"100" minimum:"1" maximum:"100"`
	Offset    int       `query:"offset" default:"0" minimum:"0"`
}

type examDocumentInput struct {
	DocumentID uuid.UUID `path:"documentId" format:"uuid"`
}

type listExamDocumentsOutput struct {
	Body []documents.ExamDocumentOutput
}

type listExamDocumentTextsOutput struct {
	Body []documents.ExamDocumentTextOutput
}

type examDocumentOutput struct {
	Body documents.ExamDocumentOutput
}

type examDocumentFileOutput struct {
	Body examDocumentFileResponse
}

type uploadExamDocumentForm struct {
	File huma.FormFile `form:"file" required:"true"`
}

type uploadExamDocumentInput struct {
	PatientID uuid.UUID `path:"patientId" format:"uuid"`
	RawBody   huma.MultipartFormFiles[uploadExamDocumentForm]
}

func NewExams(
	svc documents.Service,
	drafts draftService,
	confirmer confirmationService,
	storageClient documentprocessing.FileStorageService,
	accessChecker patientaccess.Checker,
) *ExamsHandler {
	return &ExamsHandler{svc: svc, drafts: drafts, confirmer: confirmer, storage: storageClient, accessChecker: accessChecker}
}

// RegisterHumaRoutes registers the supported exam-document operations.
func (h *ExamsHandler) RegisterHumaRoutes(registered huma.API, security []map[string][]string) {
	h.registerReviewRoutes(registered, security)
	huma.Register(registered, huma.Operation{
		OperationID: "listExamDocuments",
		Method:      http.MethodGet,
		Path:        "/patients/{patientId}/exam-documents",
		Summary:     "Listar documentos de exame",
		Tags:        []string{"Exam documents"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
		Security:    security,
	}, h.listExamDocuments)

	huma.Register(registered, huma.Operation{
		OperationID: "listExamDocumentTexts",
		Method:      http.MethodGet,
		Path:        "/patients/{patientId}/exam-document-texts",
		Summary:     "Listar textos extraídos de documentos de exame",
		Tags:        []string{"Exam documents"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
		Security:    security,
	}, h.listExamDocumentTexts)

	huma.Register(registered, huma.Operation{
		OperationID:   "uploadExamDocument",
		Method:        http.MethodPost,
		Path:          "/patients/{patientId}/exam-documents",
		Summary:       "Extrair PDF laboratorial e criar rascunho para conferência",
		MaxBodyBytes:  examDocumentMaxBodySize,
		Tags:          []string{"Exam documents"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType},
		Security:      security,
	}, h.uploadExamDocument)

	huma.Register(registered, huma.Operation{
		OperationID: "getExamDocument",
		Method:      http.MethodGet,
		Path:        "/exam-documents/{documentId}",
		Summary:     "Obter documento de exame",
		Tags:        []string{"Exam documents"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound},
		Security:    security,
	}, h.getExamDocument)

	huma.Register(registered, huma.Operation{
		OperationID: "getExamDocumentFile",
		Method:      http.MethodGet,
		Path:        "/exam-documents/{documentId}/file",
		Summary:     "Obter URL temporária do arquivo de exame",
		Tags:        []string{"Exam documents"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound},
		Security:    security,
	}, h.getExamDocumentFile)
}

func (h *ExamsHandler) listExamDocuments(ctx context.Context, input *listExamDocumentsInput) (*listExamDocumentsOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necess?ria")
	}
	if err := h.accessChecker.RequireAccess(ctx, currentAccount.ID, input.PatientID); err != nil {
		return nil, humaerror.From(err)
	}
	list, err := h.svc.ListByPatient(ctx, input.PatientID, input.Limit, input.Offset)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &listExamDocumentsOutput{Body: list}, nil
}

func (h *ExamsHandler) listExamDocumentTexts(ctx context.Context, input *listExamDocumentsInput) (*listExamDocumentTextsOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necess?ria")
	}
	if err := h.accessChecker.RequireAccess(ctx, currentAccount.ID, input.PatientID); err != nil {
		return nil, humaerror.From(err)
	}
	list, err := h.svc.ListDocumentTextsByPatient(ctx, input.PatientID, input.Limit, input.Offset)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &listExamDocumentTextsOutput{Body: list}, nil
}

func (h *ExamsHandler) getExamDocument(ctx context.Context, input *examDocumentInput) (*examDocumentOutput, error) {
	document, err := h.findAccessibleDocument(ctx, input.DocumentID)
	if err != nil {
		return nil, err
	}
	return &examDocumentOutput{Body: *document}, nil
}

func (h *ExamsHandler) getExamDocumentFile(ctx context.Context, input *examDocumentInput) (*examDocumentFileOutput, error) {
	document, err := h.findAccessibleDocument(ctx, input.DocumentID)
	if err != nil {
		return nil, err
	}
	if h.storage == nil {
		return nil, huma.Error500InternalServerError("armazenamento de documentos indisponível")
	}
	url, err := h.storage.GetSignedURL(ctx, document.StorageURI, examDocumentFileURLExpirationMinutes)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &examDocumentFileOutput{Body: examDocumentFileResponse{
		URL:       url,
		ExpiresAt: time.Now().UTC().Add(examDocumentFileURLExpirationMinutes * time.Minute),
	}}, nil
}

func (h *ExamsHandler) uploadExamDocument(ctx context.Context, input *uploadExamDocumentInput) (*examDocumentOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necess?ria")
	}
	if err := h.accessChecker.RequireAccess(ctx, currentAccount.ID, input.PatientID); err != nil {
		return nil, humaerror.From(err)
	}

	fileHeaders := input.RawBody.Form.File["file"]
	if len(fileHeaders) != 1 {
		return nil, huma.Error422UnprocessableEntity("arquivo é obrigatório")
	}
	path, err := writeTemporaryPDF(fileHeaders[0], examDocumentMaxFileSize)
	if err != nil {
		return nil, humaerror.From(err)
	}
	defer os.Remove(path)
	document, err := h.drafts.Create(ctx, documents.CreateDraftInput{PatientID: input.PatientID, UserID: currentAccount.ID, LocalPath: path, Filename: fileHeaders[0].Filename})

	if err != nil {
		return nil, humaerror.From(err)
	}
	return &examDocumentOutput{Body: *document}, nil
}

func (h *ExamsHandler) findAccessibleDocument(ctx context.Context, documentID uuid.UUID) (*documents.ExamDocumentOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necess?ria")
	}
	document, err := h.svc.FindByID(ctx, documentID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	if document == nil {
		return nil, huma.Error404NotFound("documento não encontrado")
	}
	if err := h.accessChecker.RequireAccess(ctx, currentAccount.ID, document.PatientID); err != nil {
		return nil, humaerror.From(err)
	}
	return document, nil
}
