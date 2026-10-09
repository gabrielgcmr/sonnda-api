// internal/api/capture_routes_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	capturehttp "github.com/gabrielgcmr/sonnda/internal/features/capture/http"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type captureRouteService struct {
	created       *capture.CreatedSession
	state         *capture.SessionState
	claimed       *capture.ClaimedSession
	uploaded      *capturedomain.Capture
	claimCode     string
	captureToken  string
	mobileSession uuid.UUID
	page          *capture.CapturePage
	file          *capture.SignedCaptureFile
	deleted       uuid.UUID
	inboxAccount  uuid.UUID
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

func (s *captureRouteService) ClaimSession(_ context.Context, code string) (*capture.ClaimedSession, error) {
	s.claimCode = code
	return s.claimed, nil
}

func (s *captureRouteService) AuthenticateMobile(_ context.Context, token string) (*capture.MobileCredential, error) {
	s.captureToken = token
	if token == "" {
		return nil, apperr.Unauthorized("credencial de captura necessária")
	}
	return &capture.MobileCredential{}, nil
}

func (s *captureRouteService) MobileHeartbeat(_ context.Context, _ capture.MobileCredential, sessionID uuid.UUID) (*capture.SessionState, error) {
	s.mobileSession = sessionID
	return s.state, nil
}

func (s *captureRouteService) UploadCapture(context.Context, capture.MobileCredential, capture.UploadInput) (*capturedomain.Capture, error) {
	return s.uploaded, nil
}

func (s *captureRouteService) ListCaptures(_ context.Context, accountID uuid.UUID, _ capture.Pagination) (*capture.CapturePage, error) {
	s.inboxAccount = accountID
	return s.page, nil
}

func (s *captureRouteService) GetCaptureFile(_ context.Context, accountID, _ uuid.UUID) (*capture.SignedCaptureFile, error) {
	s.inboxAccount = accountID
	return s.file, nil
}

func (s *captureRouteService) DeleteCapture(_ context.Context, accountID, captureID uuid.UUID) error {
	s.inboxAccount, s.deleted = accountID, captureID
	return nil
}

func TestCaptureClaimAndMobileRoutesUseRestrictedCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	sessionID := uuid.New()
	service := &captureRouteService{
		claimed: &capture.ClaimedSession{SessionID: sessionID, UploadToken: "upload-secret", UploadTokenExpiresAt: now.Add(12 * time.Hour)},
		state:   &capture.SessionState{ID: sessionID, PairingExpiresAt: now, DesktopLastSeenAt: now, MobilePresent: true},
		uploaded: &capturedomain.Capture{
			ID: uuid.New(), CaptureSessionID: sessionID, OriginalFilename: "exam.pdf", MIMEType: "application/pdf",
			SizeBytes: 8, Status: capturedomain.StatusAvailable, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
		},
	}
	handler := capturehttp.NewHandler(service)
	router := gin.New()
	SetupRoutes(router, &APIDependencies{
		CaptureHandler: handler,
		CaptureAuth:    capturehttp.NewMiddleware(service),
	})

	claimRequest := httptest.NewRequest(http.MethodPost, "/capture-sessions/claim", bytes.NewBufferString(`{"code":"qr-secret"}`))
	claimRequest.Header.Set("Content-Type", "application/json")
	claimResponse := httptest.NewRecorder()
	router.ServeHTTP(claimResponse, claimRequest)
	if claimResponse.Code != http.StatusOK || claimResponse.Header().Get("Cache-Control") != "private, no-store" || service.claimCode != "qr-secret" {
		t.Fatalf("claim failed: status=%d cache=%q body=%s", claimResponse.Code, claimResponse.Header().Get("Cache-Control"), claimResponse.Body.String())
	}
	if !bytes.Contains(claimResponse.Body.Bytes(), []byte(`"upload_token":"upload-secret"`)) {
		t.Fatalf("claim token missing: %s", claimResponse.Body.String())
	}

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, captureUploadRequest(t, ""))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("capture without restricted token returned %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	uploadRequest := captureUploadRequest(t, "upload-secret")
	uploadResponse := httptest.NewRecorder()
	router.ServeHTTP(uploadResponse, uploadRequest)
	if uploadResponse.Code != http.StatusCreated || service.captureToken != "upload-secret" {
		t.Fatalf("upload failed: status=%d token=%q body=%s", uploadResponse.Code, service.captureToken, uploadResponse.Body.String())
	}

	heartbeatRequest := httptest.NewRequest(http.MethodPost, "/capture-sessions/"+sessionID.String()+"/mobile-heartbeat", nil)
	heartbeatRequest.Header.Set("X-Capture-Token", "upload-secret")
	heartbeatResponse := httptest.NewRecorder()
	router.ServeHTTP(heartbeatResponse, heartbeatRequest)
	if heartbeatResponse.Code != http.StatusOK || service.mobileSession != sessionID {
		t.Fatalf("mobile heartbeat failed: status=%d body=%s", heartbeatResponse.Code, heartbeatResponse.Body.String())
	}
}

func captureUploadRequest(t *testing.T, token string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "exam.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("%PDF-1.7"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/captures", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		request.Header.Set("X-Capture-Token", token)
	}
	return request
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

func TestCaptureInboxUsesAccountScopeAndDoesNotExposeStorageURI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fullName := "Ana Silva"
	birthDate := time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)
	current, err := accountdomain.NewAccount(accountdomain.NewAccountParams{Profile: accountdomain.Profile{FullName: &fullName, BirthDate: &birthDate}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	captureID := uuid.New()
	storageURI := "supabase://captures/private/file.pdf"
	item := capturedomain.Capture{
		ID: captureID, AccountID: current.ID, CaptureSessionID: uuid.New(), StorageURI: &storageURI,
		OriginalFilename: "exam.pdf", MIMEType: "application/pdf", SizeBytes: 8,
		Status: capturedomain.StatusAvailable, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	service := &captureRouteService{
		page: &capture.CapturePage{Items: []capturedomain.Capture{item}, Limit: 20},
		file: &capture.SignedCaptureFile{URL: "https://storage.test/signed", ExpiresAt: now.Add(5 * time.Minute)},
	}
	router := gin.New()
	SetupRoutes(router, &APIDependencies{
		Auth:           authenticatedTestMiddleware(),
		Account:        accounthttp.NewMiddleware(&routeAccountService{account: current}),
		AccountHandler: accounthttp.NewHandler(&routeAccountService{account: current}, nil),
		CaptureHandler: capturehttp.NewHandler(service),
	})

	listed := accountRequest(router, http.MethodGet, "/captures", "")
	if listed.Code != http.StatusOK || bytes.Contains(listed.Body.Bytes(), []byte("storage_uri")) || bytes.Contains(listed.Body.Bytes(), []byte(storageURI)) {
		t.Fatalf("unexpected list response: status=%d body=%s", listed.Code, listed.Body.String())
	}
	file := accountRequest(router, http.MethodGet, "/captures/"+captureID.String()+"/file", "")
	if file.Code != http.StatusOK || file.Header().Get("Cache-Control") != "private, no-store" || !bytes.Contains(file.Body.Bytes(), []byte("https://storage.test/signed")) {
		t.Fatalf("unexpected file response: status=%d cache=%q body=%s", file.Code, file.Header().Get("Cache-Control"), file.Body.String())
	}
	deleted := accountRequest(router, http.MethodDelete, "/captures/"+captureID.String(), "")
	if deleted.Code != http.StatusNoContent || service.inboxAccount != current.ID || service.deleted != captureID {
		t.Fatalf("unexpected delete response: status=%d account=%s capture=%s body=%s", deleted.Code, service.inboxAccount, service.deleted, deleted.Body.String())
	}
}
