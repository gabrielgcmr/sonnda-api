// internal/features/capture/http/mobile.go
package capturehttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

const captureUploadMaxBodySize = capturedomain.MaxFileSizeBytes + 1024*1024

type claimSessionInput struct {
	Body struct {
		Code string `json:"code" minLength:"1" maxLength:"256"`
	}
}

type claimSessionResponse struct {
	SessionID            uuid.UUID `json:"session_id" format:"uuid"`
	UploadToken          string    `json:"upload_token"`
	UploadTokenExpiresAt time.Time `json:"expires_at"`
}

type claimSessionOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         claimSessionResponse
}

type captureUploadForm struct {
	File huma.FormFile `form:"file" required:"true"`
}

type captureUploadInput struct {
	RawBody huma.MultipartFormFiles[captureUploadForm]
}

type captureResponse struct {
	ID               uuid.UUID `json:"id" format:"uuid"`
	CaptureSessionID uuid.UUID `json:"capture_session_id" format:"uuid"`
	OriginalFilename string    `json:"original_filename"`
	MIMEType         string    `json:"mime_type"`
	SizeBytes        int64     `json:"size_bytes"`
	Status           string    `json:"status"`
	ExpiresAt        time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
}

type captureOutput struct {
	Body captureResponse
}

func (h *Handler) RegisterPublicRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "claimCaptureSession",
		Method:      http.MethodPost,
		Path:        "/capture-sessions/claim",
		Summary:     "Reivindicar sessão de captura",
		Tags:        []string{"Capture sessions"},
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusInternalServerError},
	}, h.claimSession)
}

func (h *Handler) RegisterMobileRoutes(api huma.API, security []map[string][]string) {
	huma.Register(api, huma.Operation{
		OperationID: "heartbeatMobileCaptureSession",
		Method:      http.MethodPost,
		Path:        "/capture-sessions/{sessionId}/mobile-heartbeat",
		Summary:     "Atualizar presença do celular",
		Tags:        []string{"Capture sessions"},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
		Security:    security,
	}, h.mobileHeartbeat)
	huma.Register(api, huma.Operation{
		OperationID:   "uploadCapture",
		Method:        http.MethodPost,
		Path:          "/captures",
		Summary:       "Enviar captura pelo celular",
		Tags:          []string{"Captures"},
		MaxBodyBytes:  captureUploadMaxBodySize,
		DefaultStatus: http.StatusCreated,
		Errors: []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusUnprocessableEntity,
			http.StatusRequestEntityTooLarge,
			http.StatusUnsupportedMediaType,
			http.StatusInternalServerError,
			http.StatusGatewayTimeout,
		},
		Security: security,
	}, h.uploadCapture)
}

func (h *Handler) claimSession(ctx context.Context, input *claimSessionInput) (*claimSessionOutput, error) {
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	claimed, err := h.service.ClaimSession(ctx, input.Body.Code)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &claimSessionOutput{
		CacheControl: captureSessionCacheControl,
		Body: claimSessionResponse{
			SessionID:            claimed.SessionID,
			UploadToken:          claimed.UploadToken,
			UploadTokenExpiresAt: claimed.UploadTokenExpiresAt,
		},
	}, nil
}

func (h *Handler) mobileHeartbeat(ctx context.Context, input *sessionPathInput) (*sessionOutput, error) {
	credential, err := mobileCredential(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	state, err := h.service.MobileHeartbeat(ctx, credential, input.SessionID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return outputFromState(state), nil
}

func (h *Handler) uploadCapture(ctx context.Context, input *captureUploadInput) (*captureOutput, error) {
	credential, err := mobileCredential(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	files := input.RawBody.Form.File["file"]
	if len(files) != 1 {
		return nil, humaerror.From(apperr.Validation("envie exatamente um arquivo", apperr.Violation{Field: "file", Reason: "exactly_one_required"}))
	}
	file, err := files[0].Open()
	if err != nil {
		return nil, humaerror.From(apperr.Internal("falha ao abrir arquivo", err))
	}
	defer file.Close()

	item, err := h.service.UploadCapture(ctx, credential, capture.UploadInput{
		File:             file,
		OriginalFilename: files[0].Filename,
		SizeBytes:        files[0].Size,
	})
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &captureOutput{Body: responseFromCapture(*item)}, nil
}

func mobileCredential(ctx context.Context) (capture.MobileCredential, error) {
	credential, ok := capture.MobileCredentialFromContext(ctx)
	if !ok {
		return capture.MobileCredential{}, humaerror.From(apperr.Internal("falha ao autenticar captura", errors.New("mobile credential is missing from request context")))
	}
	return credential, nil
}

func responseFromCapture(item capturedomain.Capture) captureResponse {
	return captureResponse{
		ID:               item.ID,
		CaptureSessionID: item.CaptureSessionID,
		OriginalFilename: item.OriginalFilename,
		MIMEType:         item.MIMEType,
		SizeBytes:        item.SizeBytes,
		Status:           string(item.Status),
		ExpiresAt:        item.ExpiresAt,
		CreatedAt:        item.CreatedAt,
	}
}
