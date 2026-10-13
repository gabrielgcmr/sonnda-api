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
	"github.com/gabrielgcmr/sonnda/internal/api/humaspec"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type cleanupService interface {
	Cleanup(ctx context.Context, opts capture.CleanupOptions) (*capture.CleanupReport, error)
}

type CleanupHandler struct {
	service          cleanupService
	token            string
	temporaryCleaner interface{ Cleanup(context.Context) error }
}

type cleanupInput struct {
	Token string `header:"X-Cleanup-Token"`
}

type cleanupResponse struct {
	RunID             uuid.UUID `json:"run_id" format:"uuid"`
	CapturesProcessed int       `json:"captures_processed"`
	CapturesDeleted   int       `json:"captures_deleted"`
	StorageDeleted    int       `json:"storage_deleted"`
	SessionsDeleted   int       `json:"sessions_deleted"`
}

type cleanupOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         cleanupResponse
}

func NewCleanupHandler(service cleanupService, token string) *CleanupHandler {
	return &CleanupHandler{service: service, token: strings.TrimSpace(token)}
}

func (h *CleanupHandler) SetTemporaryCleaner(cleaner interface{ Cleanup(context.Context) error }) {
	h.temporaryCleaner = cleaner
}

func (h *CleanupHandler) RegisterHumaRoutes(api huma.API) {
	registerCleanupRoute(api, h.cleanup)
}

// RegisterCleanupOpenAPIRoutes registers the hidden cleanup operation without runtime dependencies.
func RegisterCleanupOpenAPIRoutes(api huma.API) {
	registerCleanupRoute(api, humaspec.NotImplemented[cleanupInput, cleanupOutput])
}

func registerCleanupRoute(
	api huma.API,
	endpoint func(context.Context, *cleanupInput) (*cleanupOutput, error),
) {
	huma.Register(api, huma.Operation{
		OperationID: "cleanupCapturesJob",
		Method:      http.MethodPost,
		Path:        "/internal/jobs/cleanup-captures",
		Summary:     "Executar limpeza de capturas",
		Hidden:      true,
		Errors:      []int{http.StatusUnauthorized, http.StatusInternalServerError, http.StatusGatewayTimeout},
	}, endpoint)
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
	if h.temporaryCleaner != nil {
		if err := h.temporaryCleaner.Cleanup(ctx); err != nil {
			return nil, humaerror.From(err)
		}
	}
	return &cleanupOutput{
		CacheControl: captureSessionCacheControl,
		Body: cleanupResponse{
			RunID:             report.RunID,
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
