// internal/api/capture_routes_test.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturehttp "github.com/gabrielgcmr/sonnda/internal/features/capture/http"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type captureRouteService struct {
	created *capture.CreatedSession
	state   *capture.SessionState
}

func (s *captureRouteService) CreateSession(context.Context, uuid.UUID) (*capture.CreatedSession, error) {
	return s.created, nil
}

func (s *captureRouteService) CurrentSession(context.Context, uuid.UUID) (*capture.SessionState, error) {
	return s.state, nil
}

func (s *captureRouteService) Heartbeat(context.Context, uuid.UUID, uuid.UUID) (*capture.SessionState, error) {
	return s.state, nil
}

func (*captureRouteService) RevokeSession(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func TestCaptureSessionHTTPResponsesKeepPairingCodeOnlyOnCreation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fullName := "Ana Silva"
	birthDate := time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)
	current, err := accountdomain.NewAccount(accountdomain.NewAccountParams{Profile: accountdomain.Profile{FullName: &fullName, BirthDate: &birthDate}})
	if err != nil {
		t.Fatal(err)
	}
	accountService := &routeAccountService{account: current}
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	state := &capture.SessionState{
		ID: uuid.New(), PairingExpiresAt: now.Add(5 * time.Minute), DesktopLastSeenAt: now, DesktopPresent: true,
	}
	captureService := &captureRouteService{
		created: &capture.CreatedSession{SessionState: *state, PairingCode: "qr-secret"},
		state:   state,
	}
	router := gin.New()
	SetupRoutes(router, &APIDependencies{
		Auth:           authenticatedTestMiddleware(),
		Account:        accounthttp.NewMiddleware(accountService),
		AccountHandler: accounthttp.NewHandler(accountService, nil),
		CaptureHandler: capturehttp.NewHandler(captureService),
	})

	created := accountRequest(router, http.MethodPost, "/capture-sessions", "")
	if created.Code != http.StatusCreated || created.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("create: status=%d cache=%q body=%s", created.Code, created.Header().Get("Cache-Control"), created.Body.String())
	}
	var createdBody map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatal(err)
	}
	if createdBody["pairing_code"] != "qr-secret" || createdBody["session_id"] == nil || createdBody["pairing_code_hash"] != nil {
		t.Fatalf("unexpected create body: %s", created.Body.String())
	}

	currentResponse := accountRequest(router, http.MethodGet, "/capture-sessions/current", "")
	if currentResponse.Code != http.StatusOK || currentResponse.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("current: status=%d cache=%q body=%s", currentResponse.Code, currentResponse.Header().Get("Cache-Control"), currentResponse.Body.String())
	}
	var currentBody map[string]any
	if err := json.Unmarshal(currentResponse.Body.Bytes(), &currentBody); err != nil {
		t.Fatal(err)
	}
	if _, exists := currentBody["pairing_code"]; exists {
		t.Fatalf("pairing code leaked from current session: %s", currentResponse.Body.String())
	}
}
