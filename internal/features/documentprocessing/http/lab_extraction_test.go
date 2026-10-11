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
	domaintext "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textextraction"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type allowStandaloneAuthorizer struct{ documents.Authorizer }

func (allowStandaloneAuthorizer) AuthorizeStandalone(*accountdomain.Account, documents.Action) error {
	return nil
}

func authorizedStandalone(extractor documents.StandaloneLabExtractor) *documents.StandaloneLabExtraction {
	return documents.NewStandaloneLabExtraction(extractor, allowStandaloneAuthorizer{})
}

type standaloneTextExtractorStub struct{ path string }

func (s *standaloneTextExtractorStub) Extract(_ context.Context, input domaintext.ExtractInput) (*domaintext.ExtractOutput, error) {
	s.path = input.LocalPath
	if _, err := os.Stat(input.LocalPath); err != nil {
		return nil, err
	}
	return &domaintext.ExtractOutput{Text: "Glicose 90 mg/dL", Method: "pdf_text_raw"}, nil
}

type standaloneLabExtractorStub struct {
	called bool
	err    error
}

func (s *standaloneLabExtractorStub) ExtractLabReport(_ context.Context, input labextraction.ExtractLabReportInput) (*labextraction.ExtractedLabReport, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	value, unit := "90", "mg/dL"
	return &labextraction.ExtractedLabReport{Tests: []labextraction.ExtractedTestResult{{
		TestName: "Glicose",
		Items:    []labextraction.ExtractedTestItem{{ParameterName: "Glicose", ResultValue: &value, ResultUnit: &unit}},
	}}}, nil
}

func TestStandaloneLabExtractionDiscardsPDFAndReturnsStructuredResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	textExtractor := &standaloneTextExtractorStub{}
	labExtractor := &standaloneLabExtractorStub{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}))
		c.Next()
	})
	api := humagin.New(router, huma.DefaultConfig("test", "test"))
	NewStandaloneLabExtraction(authorizedStandalone(extraction.New(textExtractor, labExtractor)), nil).RegisterHumaRoutes(api, nil)

	body, contentType := standaloneLabMultipart(t, "exam.pdf", "application/pdf", []byte("%PDF-1.4"))
	request := httptest.NewRequest(http.MethodPost, "/lab-extractions", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if !labExtractor.called || textExtractor.path == "" {
		t.Fatal("standalone extraction did not call both extractors")
	}
	if _, err := os.Stat(textExtractor.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("standalone PDF was not removed: %v", err)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("Glicose")) {
		t.Fatalf("missing structured response: %s", response.Body.String())
	}
}

func TestStandaloneLabExtractionRejectsNonPDF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}))
		c.Next()
	})
	api := humagin.New(router, huma.DefaultConfig("test", "test"))
	NewStandaloneLabExtraction(authorizedStandalone(extraction.New(&standaloneTextExtractorStub{}, &standaloneLabExtractorStub{})), nil).RegisterHumaRoutes(api, nil)
	body, contentType := standaloneLabMultipart(t, "exam.txt", "text/plain", []byte("not a PDF"))
	request := httptest.NewRequest(http.MethodPost, "/lab-extractions", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

func TestStandaloneExtractionFailureStillRemovesPDF(t *testing.T) {
	reader := &standaloneTextExtractorStub{}
	provider := &standaloneLabExtractorStub{err: errors.New("provider unavailable")}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), &accountdomain.Account{ID: uuid.New()}))
	})
	NewStandaloneLabExtraction(authorizedStandalone(extraction.New(reader, provider)), nil).RegisterHumaRoutes(humagin.New(router, huma.DefaultConfig("test", "test")), nil)
	body, contentType := standaloneLabMultipart(t, "exam.pdf", "application/pdf", []byte("%PDF-1.4"))
	request := httptest.NewRequest(http.MethodPost, "/lab-extractions", body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code < 500 || bytes.Contains(response.Body.Bytes(), []byte("provider unavailable")) {
		t.Fatalf("unsafe failure response: %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(reader.path); !os.IsNotExist(err) {
		t.Fatal("standalone PDF leaked on failure")
	}
}

func TestStandalonePDFValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content []byte
		status  int
	}{
		{"empty", nil, 400},
		{"renamed text", []byte("not a PDF"), 400},
		{"over limit", append([]byte("%PDF-"), make([]byte, standaloneLabExtractionMaxFileSize)...), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), &accountdomain.Account{ID: uuid.New()}))
			})
			NewStandaloneLabExtraction(authorizedStandalone(extraction.New(&standaloneTextExtractorStub{}, &standaloneLabExtractorStub{})), nil).RegisterHumaRoutes(humagin.New(router, huma.DefaultConfig("test", "test")), nil)
			body, contentType := standaloneLabMultipart(t, "exam.pdf", "application/pdf", tc.content)
			request := httptest.NewRequest(http.MethodPost, "/lab-extractions", body)
			request.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

type pdfExtractorFunc func(context.Context, string, string) (*extraction.Result, error)

func (f pdfExtractorFunc) ExtractPDF(ctx context.Context, path, filename string) (*extraction.Result, error) {
	return f(ctx, path, filename)
}

func TestStandaloneLabExtractionUsesInjectedService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	want := extraction.Result{
		Status:      labextraction.ExtractionStatusPartial,
		Warnings:    []labextraction.ExtractionWarning{{Code: "review", Message: "Conferir resultado."}},
		SummaryText: "Resumo retornado pelo serviço injetado.",
	}
	user := &accountdomain.Account{ID: uuid.New()}
	requestContext := helpers.ContextWithCurrentAccount(context.Background(), user)
	var standalonePath string
	calls := 0
	extractor := pdfExtractorFunc(func(ctx context.Context, path, filename string) (*extraction.Result, error) {
		calls++
		standalonePath = path
		if ctx != requestContext || filename != "injected.pdf" {
			t.Fatalf("request context or filename changed: %s", filename)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "%PDF-1.4" {
			t.Fatalf("standalone file unavailable to injected service: %q, %v", data, err)
		}
		return &want, nil
	})
	router := gin.New()
	NewStandaloneLabExtraction(authorizedStandalone(extractor), nil).RegisterHumaRoutes(humagin.New(router, huma.DefaultConfig("test", "test")), nil)
	body, contentType := standaloneLabMultipart(t, "injected.pdf", "application/pdf", []byte("%PDF-1.4"))
	request := httptest.NewRequest(http.MethodPost, "/lab-extractions", body).WithContext(requestContext)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	var got extraction.Result
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("injected result changed: got %+v, want %+v", got, want)
	}
	if _, err := os.Stat(standalonePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("standalone PDF was not removed: %v", err)
	}
}

type denyStandaloneAuthorizer struct{ documents.Authorizer }

func (denyStandaloneAuthorizer) AuthorizeStandalone(*accountdomain.Account, documents.Action) error {
	return apperr.Forbidden("acesso negado")
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

type capturedPDFSourceStub struct {
	accountID uuid.UUID
	captureID uuid.UUID
	calls     int
	content   []byte
	body      *trackedBody
	err       error
}

func (s *capturedPDFSourceStub) OpenCapturedPDF(_ context.Context, accountID, captureID uuid.UUID) (*CapturedPDF, error) {
	s.calls++
	s.accountID, s.captureID = accountID, captureID
	if s.err != nil {
		return nil, s.err
	}
	s.body = &trackedBody{Reader: bytes.NewReader(s.content)}
	return &CapturedPDF{Body: s.body, Filename: "capturado.pdf"}, nil
}

func captureExtractionRouter(account *accountdomain.Account, extractor *documents.StandaloneLabExtraction, source CapturedPDFSource) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), account))
		c.Next()
	})
	NewStandaloneLabExtraction(extractor, source).RegisterHumaRoutes(humagin.New(router, huma.DefaultConfig("test", "test")), nil)
	return router
}

func postCaptureExtraction(router *gin.Engine, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/lab-extractions/from-capture", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestCaptureLabExtractionReadsOwnedCaptureAndReturnsStandaloneResult(t *testing.T) {
	account := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}
	captureID := uuid.New()
	want := extraction.Result{Status: labextraction.ExtractionStatusSucceeded, SummaryText: "Glicose: 90 mg/dL"}
	var extractedPath string
	extractor := pdfExtractorFunc(func(_ context.Context, path, filename string) (*extraction.Result, error) {
		extractedPath = path
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "%PDF-1.4 capture" || filename != "capturado.pdf" {
			t.Fatalf("captured file unavailable to extractor: %q %q %v", data, filename, err)
		}
		return &want, nil
	})
	source := &capturedPDFSourceStub{content: []byte("%PDF-1.4 capture")}
	router := captureExtractionRouter(account, authorizedStandalone(extractor), source)

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
	var got extraction.Result
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("result = %+v, err=%v", got, err)
	}
	if _, err := os.Stat(extractedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("captured PDF copy was not removed: %v", err)
	}
}

func TestCaptureLabExtractionFailures(t *testing.T) {
	account := &accountdomain.Account{ID: uuid.New()}
	validBody := `{"capture_id":"` + uuid.NewString() + `"}`
	for _, tc := range []struct {
		name        string
		authorizer  documents.Authorizer
		source      *capturedPDFSourceStub
		body        string
		status      int
		sourceCalls int
	}{
		{name: "capture from another account or expired", authorizer: allowStandaloneAuthorizer{}, source: &capturedPDFSourceStub{err: apperr.NotFound("captura não encontrada")}, body: validBody, status: http.StatusNotFound, sourceCalls: 1},
		{name: "capture is not a PDF", authorizer: allowStandaloneAuthorizer{}, source: &capturedPDFSourceStub{err: &apperr.AppError{Kind: apperr.UNSUPPORTED_MEDIA_TYPE, Message: "a captura não é um PDF"}}, body: validBody, status: http.StatusUnsupportedMediaType, sourceCalls: 1},
		{name: "stored content is not a PDF", authorizer: allowStandaloneAuthorizer{}, source: &capturedPDFSourceStub{content: []byte("not a PDF")}, body: validBody, status: http.StatusBadRequest, sourceCalls: 1},
		{name: "account not allowed", authorizer: denyStandaloneAuthorizer{}, source: &capturedPDFSourceStub{content: []byte("%PDF-1.4")}, body: validBody, status: http.StatusForbidden},
		{name: "invalid capture id", authorizer: allowStandaloneAuthorizer{}, source: &capturedPDFSourceStub{content: []byte("%PDF-1.4")}, body: `{"capture_id":"not-a-uuid"}`, status: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extractorCalled := false
			extractor := pdfExtractorFunc(func(context.Context, string, string) (*extraction.Result, error) {
				extractorCalled = true
				return &extraction.Result{}, nil
			})
			router := captureExtractionRouter(account, documents.NewStandaloneLabExtraction(extractor, tc.authorizer), tc.source)

			response := postCaptureExtraction(router, tc.body)

			if response.Code != tc.status || tc.source.calls != tc.sourceCalls || extractorCalled {
				t.Fatalf("status=%d sourceCalls=%d extractorCalled=%t body=%s", response.Code, tc.source.calls, extractorCalled, response.Body.String())
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
