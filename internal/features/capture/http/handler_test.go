// internal/features/capture/http/handler_test.go
package capturehttp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	"github.com/google/uuid"
)

type handlerCaptureService struct {
	created   *capture.CreatedSession
	accountID uuid.UUID
}

func (s *handlerCaptureService) CreateSession(_ context.Context, accountID uuid.UUID) (*capture.CreatedSession, error) {
	s.accountID = accountID
	return s.created, nil
}

func (*handlerCaptureService) CurrentSession(context.Context, uuid.UUID) (*capture.SessionState, error) {
	return nil, nil
}

func (*handlerCaptureService) Heartbeat(context.Context, uuid.UUID, uuid.UUID) (*capture.SessionState, error) {
	return nil, nil
}

func (*handlerCaptureService) RevokeSession(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (*handlerCaptureService) ClaimSession(context.Context, string) (*capture.ClaimedSession, error) {
	return nil, nil
}

func (*handlerCaptureService) AuthenticateMobile(context.Context, string) (*capture.MobileCredential, error) {
	return nil, nil
}

func (*handlerCaptureService) MobileHeartbeat(context.Context, capture.MobileCredential, uuid.UUID) (*capture.SessionState, error) {
	return nil, nil
}

func (*handlerCaptureService) UploadCapture(context.Context, capture.MobileCredential, capture.UploadInput) (*capturedomain.Capture, error) {
	return nil, nil
}

func TestCreateSessionReturnsSecretOnceWithNoStore(t *testing.T) {
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	account := &accountdomain.Account{ID: uuid.New()}
	service := &handlerCaptureService{created: &capture.CreatedSession{
		SessionState: capture.SessionState{ID: uuid.New(), PairingExpiresAt: now.Add(5 * time.Minute), DesktopLastSeenAt: now, DesktopPresent: true},
		PairingCode:  "qr-secret",
	}}
	handler := NewHandler(service)
	output, err := handler.createSession(helpers.ContextWithCurrentAccount(t.Context(), account), &struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if service.accountID != account.ID || output.CacheControl != captureSessionCacheControl || output.Body.PairingCode != "qr-secret" {
		t.Fatalf("unexpected output: %+v", output)
	}
	data, err := json.Marshal(output.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if body["pairing_code"] != "qr-secret" || body["session_id"] == nil || body["pairing_code_hash"] != nil || body["upload_token_hash"] != nil {
		t.Fatalf("secret or internal hash exposure mismatch: %s", data)
	}
	current := outputFromState(&service.created.SessionState)
	currentData, err := json.Marshal(current.Body)
	if err != nil {
		t.Fatal(err)
	}
	var currentBody map[string]any
	if err := json.Unmarshal(currentData, &currentBody); err != nil {
		t.Fatal(err)
	}
	if _, exists := currentBody["pairing_code"]; exists {
		t.Fatalf("pairing code leaked outside creation response: %s", currentData)
	}
}
