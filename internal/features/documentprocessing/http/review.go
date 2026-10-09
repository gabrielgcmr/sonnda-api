// internal/features/documentprocessing/http/review.go
package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	laboratory "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type extractionOutput struct{ Body extraction.Result }
type confirmationOutput struct{ Body laboratory.LabReportOutput }

func (h *ExamsHandler) registerReviewRoutes(api huma.API, security []map[string][]string) {
	huma.Register(api, huma.Operation{OperationID: "getExamDocumentExtraction", Method: http.MethodGet, Path: "/exam-documents/{documentId}/extraction", Summary: "Conferir extração do documento", Tags: []string{"Exam documents"}, Security: security, Errors: []int{401, 403, 404}}, h.getExtraction)
	huma.Register(api, huma.Operation{OperationID: "confirmExamDocument", Method: http.MethodPost, Path: "/exam-documents/{documentId}/confirmation", Summary: "Confirmar exame no histórico", Tags: []string{"Exam documents"}, Security: security, Errors: []int{401, 403, 404, 409, 422}}, h.confirm)
	huma.Register(api, huma.Operation{OperationID: "discardExamDocument", Method: http.MethodDelete, Path: "/exam-documents/{documentId}", Summary: "Excluir rascunho e PDF", DefaultStatus: http.StatusNoContent, Tags: []string{"Exam documents"}, Security: security, Errors: []int{401, 403, 404, 409}}, h.discard)
}

func (h *ExamsHandler) getExtraction(ctx context.Context, input *examDocumentInput) (*extractionOutput, error) {
	user, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}
	result, err := h.drafts.Extraction(ctx, user, input.DocumentID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &extractionOutput{Body: *result}, nil
}

func (h *ExamsHandler) confirm(ctx context.Context, input *examDocumentInput) (*confirmationOutput, error) {
	user, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}
	report, err := h.confirmer.Confirm(ctx, user, input.DocumentID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &confirmationOutput{Body: *report}, nil
}

func (h *ExamsHandler) discard(ctx context.Context, input *examDocumentInput) (*struct{}, error) {
	user, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}
	if err := h.drafts.Delete(ctx, user, input.DocumentID); err != nil {
		return nil, humaerror.From(err)
	}
	return nil, nil
}
