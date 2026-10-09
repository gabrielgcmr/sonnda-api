// internal/features/capture/http/cleanup.go
package capturehttp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type cleanupService interface {
	Cleanup(ctx context.Context, opts capture.CleanupOptions) (*capture.CleanupReport, error)
}

type CleanupHandler struct {
	service cleanupService
	token   string
}

type cleanupInput struct {
	Token string `header:"X-Cleanup-Token"`
}

type cleanupResponse struct {
	CapturesProcessed int `json:"captures_processed"`
	CapturesDeleted   int `json:"captures_deleted"`
	StorageDeleted    int `json:"storage_deleted"`
	SessionsDeleted   int `json:"sessions_deleted"`
}

type cleanupOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         cleanupResponse
}

func NewCleanupHandler(service cleanupService, token string) *CleanupHandler {
	return &CleanupHandler{service: service, token: strings.TrimSpace(token)}
}

func (h *CleanupHandler) RegisterHumaRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "cleanupCapturesJob",
		Method:      http.MethodPost,
		Path:        "/internal/jobs/cleanup-captures",
		Summary:     "Executar limpeza de capturas",
		Hidden:      true,
		Errors:      []int{http.StatusUnauthorized, http.StatusInternalServerError, http.StatusGatewayTimeout},
	}, h.cleanup)
}

func (h *CleanupHandler) cleanup(ctx context.Context, input *cleanupInput) (*cleanupOutput, error) {
	if h == nil || h.service == nil || h.token == "" {
		return nil, humaerror.From(apperr.Internal("limpeza de capturas indisponível", errors.New("capture cleanup handler is not configured")))
	}
	if !secureTokenEqual(h.token, input.Token) {
		return nil, humaerror.From(apperr.Unauthorized("credencial do job inválida"))
	}
	report, err := h.service.Cleanup(ctx, capture.CleanupOptions{})
	if err != nil {
		return nil, humaerror.From(err)
	}
	if len(report.Errors) > 0 {
		return nil, humaerror.From(apperr.Internal("limpeza de capturas incompleta", errors.Join(report.Errors...)))
	}
	return &cleanupOutput{
		CacheControl: captureSessionCacheControl,
		Body: cleanupResponse{
			CapturesProcessed: report.CapturesProcessed,
			CapturesDeleted:   report.CapturesDeleted,
			StorageDeleted:    report.StorageDeleted,
			SessionsDeleted:   report.SessionsDeleted,
		},
	}, nil
}

func secureTokenEqual(expected, provided string) bool {
	expectedHash := sha256.Sum256([]byte(expected))
	providedHash := sha256.Sum256([]byte(strings.TrimSpace(provided)))
	return expected != "" && provided != "" && subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) == 1
}
