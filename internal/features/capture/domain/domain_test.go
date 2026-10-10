// internal/features/capture/domain/domain_test.go
package capturedomain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSessionLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.FixedZone("test", -3*60*60))
	session, err := NewSession(uuid.New(), make([]byte, CredentialHashBytes), now)
	if err != nil {
		t.Fatal(err)
	}
	if session.CreatedAt.Location() != time.UTC || !session.PairingExpiresAt.Equal(session.CreatedAt.Add(MaxPairingLifetime)) {
		t.Fatalf("unexpected session timestamps: %+v", session)
	}
	claimed, err := session.Claim(make([]byte, CredentialHashBytes), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ClaimedAt == nil || claimed.UploadTokenExpiresAt == nil ||
		!claimed.UploadTokenExpiresAt.Equal(claimed.ClaimedAt.Add(MaxUploadLifetime)) {
		t.Fatalf("unexpected claim: %+v", claimed)
	}
	if _, err = claimed.Claim(make([]byte, CredentialHashBytes), now.Add(2*time.Minute)); !errors.Is(err, ErrInvalidClaim) {
		t.Fatalf("second claim error = %v", err)
	}
}

func TestCaptureLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	capture, err := NewCapture(NewCaptureParams{
		AccountID: uuid.New(), CaptureSessionID: uuid.New(), OriginalFilename: " exame.pdf ",
		MIMEType: "APPLICATION/PDF", SizeBytes: 100, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if capture.OriginalFilename != "exame.pdf" || capture.MIMEType != "application/pdf" ||
		capture.Status != StatusUploading || !capture.ExpiresAt.Equal(now.Add(RetentionPeriod)) {
		t.Fatalf("unexpected capture: %+v", capture)
	}
	available, err := capture.MakeAvailable(" gs://private/capture ", now.Add(time.Second))
	if err != nil || available.StorageURI == nil || *available.StorageURI != "gs://private/capture" {
		t.Fatalf("make available: %+v %v", available, err)
	}
	deleting, err := available.MarkDeleting(now.Add(2 * time.Second))
	if err != nil || deleting.Status != StatusDeleting {
		t.Fatalf("mark deleting: %+v %v", deleting, err)
	}
}

func TestCaptureReservesStorageURIBeforeUpload(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	item, err := NewCapture(NewCaptureParams{
		AccountID: uuid.New(), CaptureSessionID: uuid.New(), OriginalFilename: "exam.pdf",
		MIMEType: "application/pdf", SizeBytes: 128, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := item.ReserveStorageURI("supabase://captures/account/capture.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if reserved.StorageURI == nil || *reserved.StorageURI != "supabase://captures/account/capture.pdf" || reserved.Status != StatusUploading {
		t.Fatalf("unexpected reserved capture: %+v", reserved)
	}
	if _, err := reserved.ReserveStorageURI("supabase://captures/account/other.pdf"); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("second reservation error = %v", err)
	}
}

func TestCaptureRejectsUnsupportedOrOversizedFile(t *testing.T) {
	base := NewCaptureParams{
		AccountID: uuid.New(), CaptureSessionID: uuid.New(), OriginalFilename: "file.txt",
		MIMEType: "text/plain", SizeBytes: 1, CreatedAt: time.Now(),
	}
	if _, err := NewCapture(base); !errors.Is(err, ErrInvalidMIMEType) {
		t.Fatalf("MIME error = %v", err)
	}
	base.MIMEType = "image/png"
	base.SizeBytes = MaxFileSizeBytes + 1
	if _, err := NewCapture(base); !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("size error = %v", err)
	}
}
