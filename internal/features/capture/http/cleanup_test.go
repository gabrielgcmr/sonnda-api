// internal/features/capture/http/cleanup_test.go
package capturehttp

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
)

type cleanupServiceStub struct {
	report *capture.CleanupReport
	err    error
	calls  int
}

func (s *cleanupServiceStub) Cleanup(context.Context, capture.CleanupOptions) (*capture.CleanupReport, error) {
	s.calls++
	return s.report, s.err
}

func TestCleanupHandlerRejectsInvalidToken(t *testing.T) {
	for _, test := range []struct {
		name  string
		token string
	}{
		{name: "missing"},
		{name: "different", token: "different-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &cleanupServiceStub{}
			handler := NewCleanupHandler(service, "expected-token")

			_, err := handler.cleanup(t.Context(), &cleanupInput{Token: test.token})
			assertCleanupStatus(t, err, http.StatusUnauthorized)
			if service.calls != 0 {
				t.Fatalf("cleanup calls = %d, want 0", service.calls)
			}
		})
	}
}

func TestCleanupHandlerReturnsReport(t *testing.T) {
	service := &cleanupServiceStub{report: &capture.CleanupReport{
		CapturesProcessed: 4,
		CapturesDeleted:   3,
		StorageDeleted:    2,
		SessionsDeleted:   1,
	}}
	handler := NewCleanupHandler(service, "expected-token")

	output, err := handler.cleanup(t.Context(), &cleanupInput{Token: " expected-token "})
	if err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	if service.calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", service.calls)
	}
	if output.CacheControl != captureSessionCacheControl {
		t.Fatalf("cache control = %q, want %q", output.CacheControl, captureSessionCacheControl)
	}
	if output.Body.CapturesProcessed != 4 || output.Body.CapturesDeleted != 3 || output.Body.StorageDeleted != 2 || output.Body.SessionsDeleted != 1 {
		t.Fatalf("unexpected cleanup response: %#v", output.Body)
	}
}

func TestCleanupHandlerReportsPartialFailure(t *testing.T) {
	service := &cleanupServiceStub{report: &capture.CleanupReport{
		Errors: []error{errors.New("storage unavailable")},
	}}
	handler := NewCleanupHandler(service, "expected-token")

	_, err := handler.cleanup(t.Context(), &cleanupInput{Token: "expected-token"})
	assertCleanupStatus(t, err, http.StatusInternalServerError)
}

func assertCleanupStatus(t *testing.T, err error, expected int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected HTTP status %d, got nil error", expected)
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error %T does not implement huma.StatusError", err)
	}
	if statusErr.GetStatus() != expected {
		t.Fatalf("status = %d, want %d", statusErr.GetStatus(), expected)
	}
}
