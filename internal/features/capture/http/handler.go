// internal/features/capture/http/handler.go
package capturehttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

const captureSessionCacheControl = "private, no-store"

type captureService interface {
	CreateSession(ctx context.Context, accountID uuid.UUID) (*capture.CreatedSession, error)
	CurrentSession(ctx context.Context, accountID uuid.UUID) (*capture.SessionState, error)
	Heartbeat(ctx context.Context, accountID, sessionID uuid.UUID) (*capture.SessionState, error)
	RevokeSession(ctx context.Context, accountID, sessionID uuid.UUID) error
}

type Handler struct {
	service captureService
}

type sessionPathInput struct {
	SessionID uuid.UUID `path:"sessionId" format:"uuid"`
}

type sessionResponse struct {
	SessionID            uuid.UUID  `json:"session_id" format:"uuid"`
	PairingExpiresAt     time.Time  `json:"pairing_expires_at"`
	ClaimedAt            *time.Time `json:"claimed_at" nullable:"true"`
	UploadTokenExpiresAt *time.Time `json:"upload_token_expires_at" nullable:"true"`
	DesktopLastSeenAt    time.Time  `json:"desktop_last_seen_at"`
	MobileLastSeenAt     *time.Time `json:"mobile_last_seen_at" nullable:"true"`
	DesktopPresent       bool       `json:"desktop_present"`
	MobilePresent        bool       `json:"mobile_present"`
	Connected            bool       `json:"connected"`
}

type createSessionResponse struct {
	SessionID            uuid.UUID  `json:"session_id" format:"uuid"`
	PairingCode          string     `json:"pairing_code"`
	PairingExpiresAt     time.Time  `json:"pairing_expires_at"`
	ClaimedAt            *time.Time `json:"claimed_at" nullable:"true"`
	UploadTokenExpiresAt *time.Time `json:"upload_token_expires_at" nullable:"true"`
	DesktopLastSeenAt    time.Time  `json:"desktop_last_seen_at"`
	MobileLastSeenAt     *time.Time `json:"mobile_last_seen_at" nullable:"true"`
	DesktopPresent       bool       `json:"desktop_present"`
	MobilePresent        bool       `json:"mobile_present"`
	Connected            bool       `json:"connected"`
}

type sessionOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         sessionResponse
}

type createSessionOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         createSessionResponse
}

func NewHandler(service captureService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterHumaRoutes(api huma.API, security []map[string][]string) {
	huma.Register(api, huma.Operation{
		OperationID: "createCaptureSession", Method: http.MethodPost, Path: "/capture-sessions",
		Summary: "Criar sessão de captura", Tags: []string{"Capture sessions"},
		DefaultStatus: http.StatusCreated, Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError}, Security: security,
	}, h.createSession)
	huma.Register(api, huma.Operation{
		OperationID: "getCurrentCaptureSession", Method: http.MethodGet, Path: "/capture-sessions/current",
		Summary: "Obter sessão de captura atual", Tags: []string{"Capture sessions"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}, Security: security,
	}, h.currentSession)
	huma.Register(api, huma.Operation{
		OperationID: "heartbeatCaptureSession", Method: http.MethodPost, Path: "/capture-sessions/{sessionId}/heartbeat",
		Summary: "Atualizar presença do computador", Tags: []string{"Capture sessions"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}, Security: security,
	}, h.heartbeat)
	huma.Register(api, huma.Operation{
		OperationID: "revokeCaptureSession", Method: http.MethodDelete, Path: "/capture-sessions/{sessionId}",
		Summary: "Revogar sessão de captura", Tags: []string{"Capture sessions"}, DefaultStatus: http.StatusNoContent,
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}, Security: security,
	}, h.revokeSession)
}

func (h *Handler) createSession(ctx context.Context, _ *struct{}) (*createSessionOutput, error) {
	accountID, err := currentAccountID(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	created, err := h.service.CreateSession(ctx, accountID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &createSessionOutput{
		CacheControl: captureSessionCacheControl,
		Body:         createResponseFromCreated(created),
	}, nil
}

func createResponseFromCreated(created *capture.CreatedSession) createSessionResponse {
	return createSessionResponse{
		SessionID: created.ID, PairingCode: created.PairingCode, PairingExpiresAt: created.PairingExpiresAt,
		ClaimedAt: created.ClaimedAt, UploadTokenExpiresAt: created.UploadTokenExpiresAt,
		DesktopLastSeenAt: created.DesktopLastSeenAt, MobileLastSeenAt: created.MobileLastSeenAt,
		DesktopPresent: created.DesktopPresent, MobilePresent: created.MobilePresent, Connected: created.Connected,
	}
}

func (h *Handler) currentSession(ctx context.Context, _ *struct{}) (*sessionOutput, error) {
	accountID, err := currentAccountID(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	state, err := h.service.CurrentSession(ctx, accountID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return outputFromState(state), nil
}

func (h *Handler) heartbeat(ctx context.Context, input *sessionPathInput) (*sessionOutput, error) {
	accountID, err := currentAccountID(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	state, err := h.service.Heartbeat(ctx, accountID, input.SessionID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return outputFromState(state), nil
}

func (h *Handler) revokeSession(ctx context.Context, input *sessionPathInput) (*struct{}, error) {
	accountID, err := currentAccountID(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.service == nil {
		return nil, unavailable()
	}
	if err := h.service.RevokeSession(ctx, accountID, input.SessionID); err != nil {
		return nil, humaerror.From(err)
	}
	return &struct{}{}, nil
}

func currentAccountID(ctx context.Context) (uuid.UUID, error) {
	account, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return uuid.Nil, humaerror.From(apperr.Internal("falha ao resolver conta", errors.New("current account is missing from request context")))
	}
	return account.ID, nil
}

func unavailable() error {
	return humaerror.From(apperr.Internal("sessões de captura indisponíveis", errors.New("capture service is not configured")))
}

func outputFromState(state *capture.SessionState) *sessionOutput {
	return &sessionOutput{CacheControl: captureSessionCacheControl, Body: responseFromState(*state)}
}

func responseFromState(state capture.SessionState) sessionResponse {
	return sessionResponse{
		SessionID: state.ID, PairingExpiresAt: state.PairingExpiresAt, ClaimedAt: state.ClaimedAt,
		UploadTokenExpiresAt: state.UploadTokenExpiresAt, DesktopLastSeenAt: state.DesktopLastSeenAt,
		MobileLastSeenAt: state.MobileLastSeenAt, DesktopPresent: state.DesktopPresent,
		MobilePresent: state.MobilePresent, Connected: state.Connected,
	}
}
