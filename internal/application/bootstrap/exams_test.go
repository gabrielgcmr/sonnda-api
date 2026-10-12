// internal/application/bootstrap/exams_test.go
package bootstrap

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/google/uuid"
)

type captureFileSourceStub struct {
	file *capture.CaptureFile
}

func (s captureFileSourceStub) OpenCaptureFile(context.Context, uuid.UUID, uuid.UUID) (*capture.CaptureFile, error) {
	return s.file, nil
}

type trackedReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackedReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestCapturedFileSourcePreservesImageMetadata(t *testing.T) {
	body := &trackedReadCloser{Reader: bytes.NewReader([]byte{0xff, 0xd8, 0xff})}
	source := capturedFileSource{captures: captureFileSourceStub{file: &capture.CaptureFile{
		Body: body, OriginalFilename: "foto.jpg", MIMEType: "image/jpeg", SizeBytes: 3,
	}}}

	file, err := source.OpenCapturedFile(t.Context(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if file.Body != body || file.Filename != "foto.jpg" || file.MIMEType != "image/jpeg" || file.SizeBytes != 3 {
		t.Fatalf("capture metadata changed: %+v", file)
	}
	if body.closed {
		t.Fatal("adapter closed a body owned by the handler")
	}
}
