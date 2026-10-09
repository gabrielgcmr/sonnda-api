// internal/features/capture/http/inbox.go
package capturehttp

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/google/uuid"
)

type captureListInput struct {
	Limit  int `query:"limit" default:"20" minimum:"1" maximum:"100"`
	Offset int `query:"offset" default:"0" minimum:"0" maximum:"2147483647"`
}

type capturePathInput struct {
	CaptureID uuid.UUID `path:"captureId" format:"uuid"`
}

type captureListResponse struct {
	Items   []captureResponse `json:"items"`
	Limit   int               `json:"limit"`
	Offset  int               `json:"offset"`
	HasMore bool              `json:"has_more"`
}

type captureListOutput struct {
	Body captureListResponse
}

type captureFileResponse struct {
	URL       string    `json:"url" format:"uri"`
	ExpiresAt time.Time `json:"expires_at"`
}

type captureFileOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         captureFileResponse
}

func (h *Handler) registerInboxRoutes(api huma.API, security []map[string][]string) {
	huma.Register(api, huma.Operation{
		OperationID: "listCaptures",
		Method:      http.MethodGet,
		Path:        "/captures",
		Summary:     "Listar capturas disponíveis",
		Description: "Retorna capturas não expiradas da conta, da mais recente para a mais antiga.",
		Tags:        []string{"Captures"},
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError},
		Security:    security,
	}, h.listCaptures)
	huma.Register(api, huma.Operation{
		OperationID: "getCaptureFile",
		Method:      http.MethodGet,
		Path:        "/captures/{captureId}/file",
		Summary:     "Obter URL temporária da captura",
		Description: "A URL assinada expira em até cinco minutos e nunca depois da captura.",
		Tags:        []string{"Captures"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError, http.StatusGatewayTimeout},
		Security:    security,
	}, h.getCaptureFile)
	huma.Register(api, huma.Operation{
		OperationID:   "deleteCapture",
		Method:        http.MethodDelete,
		Path:          "/captures/{captureId}",
		Summary:       "Excluir captura",
		Description:   "A operação é idempotente e também retoma exclusões interrompidas.",
		Tags:          []string{"Captures"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError, http.StatusGatewayTimeout},
		Security:      security,
	}, h.deleteCapture)
}

func (h *Handler) listCaptures(ctx context.Context, input *captureListInput) (*captureListOutput, error) {
	accountID, err := currentAccountID(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	page, err := h.service.ListCaptures(ctx, accountID, capture.Pagination{Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return nil, humaerror.From(err)
	}
	items := make([]captureResponse, len(page.Items))
	for i, item := range page.Items {
		items[i] = responseFromCapture(item)
	}
	return &captureListOutput{Body: captureListResponse{
		Items: items, Limit: page.Limit, Offset: page.Offset, HasMore: page.HasMore,
	}}, nil
}

func (h *Handler) getCaptureFile(ctx context.Context, input *capturePathInput) (*captureFileOutput, error) {
	accountID, err := currentAccountID(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	file, err := h.service.GetCaptureFile(ctx, accountID, input.CaptureID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &captureFileOutput{
		CacheControl: captureSessionCacheControl,
		Body:         captureFileResponse{URL: file.URL, ExpiresAt: file.ExpiresAt},
	}, nil
}

func (h *Handler) deleteCapture(ctx context.Context, input *capturePathInput) (*struct{}, error) {
	accountID, err := currentAccountID(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	if err := h.service.DeleteCapture(ctx, accountID, input.CaptureID); err != nil {
		return nil, humaerror.From(err)
	}
	return &struct{}{}, nil
}
