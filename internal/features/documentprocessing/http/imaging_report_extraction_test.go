// internal/features/documentprocessing/http/imaging_report_extraction_test.go
package http

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type imagingExtractionStub struct {
	accountID uuid.UUID
	pages     []imagingreport.DocumentPage
}

func TestImagingPageUploadAcceptsOrderedImages(t *testing.T) {
	account := &accountdomain.Account{ID: uuid.New()}
	extractor := &imagingExtractionStub{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), account))
		c.Next()
	})
	api := humagin.New(router, huma.DefaultConfig("test", "test"))
	NewStandaloneImagingExtraction(extractor, nil).RegisterHumaRoutes(api, nil)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, item := range []struct {
		name string
		data []byte
	}{
		{"page-1.jpg", []byte{0xff, 0xd8, 0xff, 0xe0, 0x01}},
		{"page-2.png", []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x02}},
	} {
		part, err := writer.CreateFormFile("files", item.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(item.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/imaging-report-extractions/pages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if extractor.accountID != account.ID || len(extractor.pages) != 2 || extractor.pages[0].Filename != "page-1.jpg" || extractor.pages[1].Filename != "page-2.png" {
		t.Fatalf("page order or account lost: account=%s pages=%+v", extractor.accountID, extractor.pages)
	}
	for _, page := range extractor.pages {
		if _, err := os.Stat(page.Path); !os.IsNotExist(err) {
			t.Fatalf("temporary page was retained: %s, err=%v", page.Path, err)
		}
	}
}

func (s *imagingExtractionStub) Authorize(*accountdomain.Account) error { return nil }

func (s *imagingExtractionStub) ExtractDocument(_ context.Context, account *accountdomain.Account, _, _, _ string) (*imagingreport.ImagingReportResult, error) {
	s.accountID = account.ID
	return &imagingreport.ImagingReportResult{Status: "succeeded"}, nil
}

func (s *imagingExtractionStub) ExtractImagePages(_ context.Context, account *accountdomain.Account, pages []imagingreport.DocumentPage) (*imagingreport.ImagingReportResult, error) {
	s.accountID = account.ID
	s.pages = pages
	return &imagingreport.ImagingReportResult{Status: "succeeded"}, nil
}

type capturedImagingFileStub struct {
	accountID  uuid.UUID
	accountIDs []uuid.UUID
}

func (s *capturedImagingFileStub) OpenCapturedFile(_ context.Context, accountID, _ uuid.UUID) (*CapturedFile, error) {
	s.accountID = accountID
	s.accountIDs = append(s.accountIDs, accountID)
	return &CapturedFile{Body: io.NopCloser(bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xe0})), Filename: "capture.jpg", MIMEType: "image/jpeg"}, nil
}

func TestImagingExtractionFromCapturesScopesEveryPageToAccount(t *testing.T) {
	account := &accountdomain.Account{ID: uuid.New()}
	captures := &capturedImagingFileStub{}
	extractor := &imagingExtractionStub{}
	handler := NewStandaloneImagingExtraction(extractor, captures)
	ctx := helpers.ContextWithCurrentAccount(t.Context(), account)
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	_, err := handler.extractFromCaptures(ctx, &imagingCapturesInput{Body: imagingCapturesRequest{CaptureIDs: ids}})
	if err != nil {
		t.Fatal(err)
	}
	if len(captures.accountIDs) != 2 || len(extractor.pages) != 2 || captures.accountIDs[0] != account.ID || captures.accountIDs[1] != account.ID || extractor.accountID != account.ID {
		t.Fatalf("multi-page extraction escaped account scope: source=%v extractor=%s", captures.accountIDs, extractor.accountID)
	}
	_, err = handler.extractFromCaptures(ctx, &imagingCapturesInput{Body: imagingCapturesRequest{CaptureIDs: []uuid.UUID{ids[0], ids[0]}}})
	if err == nil {
		t.Fatal("duplicate capture was accepted")
	}
}

func TestImagingExtractionFromCaptureIsScopedToAuthenticatedAccount(t *testing.T) {
	account := &accountdomain.Account{ID: uuid.New()}
	captures := &capturedImagingFileStub{}
	extractor := &imagingExtractionStub{}
	handler := NewStandaloneImagingExtraction(extractor, captures)
	ctx := helpers.ContextWithCurrentAccount(t.Context(), account)
	_, err := handler.extractFromCapture(ctx, &imagingCaptureInput{Body: imagingCaptureRequest{CaptureID: uuid.New()}})
	if err != nil {
		t.Fatal(err)
	}
	if captures.accountID != account.ID || extractor.accountID != account.ID {
		t.Fatalf("capture processing escaped account scope: source=%s extractor=%s account=%s", captures.accountID, extractor.accountID, account.ID)
	}
}
