// internal/features/documentprocessing/http/lab_extraction_test.go
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	documents "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labextraction"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type allowStandaloneAuthorizer struct{ documents.Authorizer }

func (allowStandaloneAuthorizer) AuthorizeStandalone(*accountdomain.Account, documents.Action) error {
	return nil
}

type denyStandaloneAuthorizer struct{ documents.Authorizer }

func (denyStandaloneAuthorizer) AuthorizeStandalone(*accountdomain.Account, documents.Action) error {
	return apperr.Forbidden("acesso negado")
}

type documentExtractorFunc func(context.Context, string, string, string) (*extraction.Result, error)

func (f documentExtractorFunc) ExtractDocument(ctx context.Context, path, filename, mimeType string) (*extraction.Result, error) {
	return f(ctx, path, filename, mimeType)
}

func authorizedStandalone(extractor documents.StandaloneLabExtractor) *documents.StandaloneLabExtraction {
	return documents.NewStandaloneLabExtraction(extractor, allowStandaloneAuthorizer{})
}

func standaloneRouter(account *accountdomain.Account, extractor *documents.StandaloneLabExtraction, source CapturedFileSource) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), account))
		c.Next()
	})
	api := humagin.New(router, huma.DefaultConfig("test", "test"))
	NewStandaloneLabExtraction(extractor, source).RegisterHumaRoutes(api, nil)
	return router
}

func TestStandaloneLabExtractionAcceptsPDFJPEGAndPNG(t *testing.T) {
	formats := []struct {
		name     string
		filename string
		mimeType string
		content  []byte
	}{
		{name: "pdf", filename: "exam.pdf", mimeType: "application/pdf", content: []byte("%PDF-1.4")},
		{name: "jpeg", filename: "exam.jpg", mimeType: "image/jpeg", content: []byte{0xff, 0xd8, 0xff, 0x01}},
		{name: "png", filename: "exam.png", mimeType: "image/png", content: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}},
	}
	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			want := extraction.Result{Status: labextraction.ExtractionStatusSucceeded, SummaryText: "Glicose: 90 mg/dL"}
			var temporaryPath string
			extractor := documentExtractorFunc(func(_ context.Context, path, filename, mimeType string) (*extraction.Result, error) {
				temporaryPath = path
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, tc.content) || filename != tc.filename || mimeType != tc.mimeType {
					t.Fatalf("document mismatch: data=%x filename=%q mime=%q err=%v", data, filename, mimeType, err)
				}
				return &want, nil
			})
			router := standaloneRouter(&accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}, authorizedStandalone(extractor), nil)
			body, contentType := standaloneLabMultipart(t, tc.filename, tc.mimeType, tc.content)
			request := httptest.NewRequest(http.MethodPost, "/lab-extractions", body)
			request.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			var got extraction.Result
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("result=%+v err=%v", got, err)
			}
			if _, err := os.Stat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary document was not removed: %v", err)
			}
		})
	}
}

func TestStandaloneDocumentValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content []byte
		status  int
	}{
		{name: "empty", content: nil, status: http.StatusBadRequest},
		{name: "unsupported signature", content: []byte("plain text"), status: http.StatusUnsupportedMediaType},
		{name: "over limit", content: append([]byte("%PDF-"), make([]byte, standaloneLabExtractionMaxFileSize)...), status: http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			extractor := documentExtractorFunc(func(context.Context, string, string, string) (*extraction.Result, error) {
				called = true
				return &extraction.Result{}, nil
			})
			router := standaloneRouter(&accountdomain.Account{ID: uuid.New()}, authorizedStandalone(extractor), nil)
			body, contentType := standaloneLabMultipart(t, "exam.pdf", "application/pdf", tc.content)
			request := httptest.NewRequest(http.MethodPost, "/lab-extractions", body)
			request.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status || called {
				t.Fatalf("status=%d called=%t body=%s", response.Code, called, response.Body.String())
			}
		})
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

type capturedFileSourceStub struct {
	accountID uuid.UUID
	captureID uuid.UUID
	calls     int
	content   []byte
	mimeType  string
	filename  string
	body      *trackedBody
	err       error
}

func (s *capturedFileSourceStub) OpenCapturedFile(_ context.Context, accountID, captureID uuid.UUID) (*CapturedFile, error) {
	s.calls++
	s.accountID, s.captureID = accountID, captureID
	if s.err != nil {
		return nil, s.err
	}
	s.body = &trackedBody{Reader: bytes.NewReader(s.content)}
	return &CapturedFile{Body: s.body, Filename: s.filename, MIMEType: s.mimeType, SizeBytes: int64(len(s.content))}, nil
}

func postCaptureExtraction(router *gin.Engine, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/lab-extractions/from-capture", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestCaptureLabExtractionForwardsOwnedImageAndClosesBody(t *testing.T) {
	account := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}
	captureID := uuid.New()
	content := []byte{0xff, 0xd8, 0xff, 0x01}
	want := extraction.Result{Status: labextraction.ExtractionStatusSucceeded, SummaryText: "Glicose: 90 mg/dL"}
	var extractedPath string
	extractor := documentExtractorFunc(func(_ context.Context, path, filename, mimeType string) (*extraction.Result, error) {
		extractedPath = path
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, content) || filename != "capturado.jpg" || mimeType != "image/jpeg" {
			t.Fatalf("capture mismatch: data=%x filename=%q mime=%q err=%v", data, filename, mimeType, err)
		}
		return &want, nil
	})
	source := &capturedFileSourceStub{content: content, mimeType: "image/jpeg", filename: "capturado.jpg"}
	router := standaloneRouter(account, authorizedStandalone(extractor), source)

	response := postCaptureExtraction(router, `{"capture_id":"`+captureID.String()+`"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if source.accountID != account.ID || source.captureID != captureID {
		t.Fatalf("capture lookup not scoped to account: account=%s capture=%s", source.accountID, source.captureID)
	}
	if !source.body.closed {
		t.Fatal("capture body was not closed")
	}
	if _, err := os.Stat(extractedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary capture was not removed: %v", err)
	}
}

func TestCaptureLabExtractionProtectsAccountAndContentBoundaries(t *testing.T) {
	account := &accountdomain.Account{ID: uuid.New()}
	validBody := `{"capture_id":"` + uuid.NewString() + `"}`
	for _, tc := range []struct {
		name        string
		authorizer  documents.Authorizer
		source      *capturedFileSourceStub
		body        string
		status      int
		sourceCalls int
	}{
		{name: "capture from another account or expired", authorizer: allowStandaloneAuthorizer{}, source: &capturedFileSourceStub{err: apperr.NotFound("captura não encontrada")}, body: validBody, status: http.StatusNotFound, sourceCalls: 1},
		{name: "stored signature mismatches MIME", authorizer: allowStandaloneAuthorizer{}, source: &capturedFileSourceStub{content: []byte("%PDF-1.4"), mimeType: "image/jpeg", filename: "foto.jpg"}, body: validBody, status: http.StatusUnsupportedMediaType, sourceCalls: 1},
		{name: "unsupported stored content", authorizer: allowStandaloneAuthorizer{}, source: &capturedFileSourceStub{content: []byte("text"), mimeType: "text/plain", filename: "note.txt"}, body: validBody, status: http.StatusUnsupportedMediaType, sourceCalls: 1},
		{name: "account not allowed", authorizer: denyStandaloneAuthorizer{}, source: &capturedFileSourceStub{content: []byte("%PDF-1.4"), mimeType: "application/pdf", filename: "exam.pdf"}, body: validBody, status: http.StatusForbidden},
		{name: "invalid capture id", authorizer: allowStandaloneAuthorizer{}, source: &capturedFileSourceStub{}, body: `{"capture_id":"not-a-uuid"}`, status: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			extractor := documentExtractorFunc(func(context.Context, string, string, string) (*extraction.Result, error) {
				called = true
				return &extraction.Result{}, nil
			})
			router := standaloneRouter(account, documents.NewStandaloneLabExtraction(extractor, tc.authorizer), tc.source)
			response := postCaptureExtraction(router, tc.body)
			if response.Code != tc.status || tc.source.calls != tc.sourceCalls || called {
				t.Fatalf("status=%d sourceCalls=%d called=%t body=%s", response.Code, tc.source.calls, called, response.Body.String())
			}
			if tc.source.body != nil && !tc.source.body.closed {
				t.Fatal("capture body was not closed")
			}
		})
	}
}

func standaloneLabMultipart(t *testing.T, filename, _ string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}
