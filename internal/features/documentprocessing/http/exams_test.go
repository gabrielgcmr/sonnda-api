// internal/features/documentprocessing/http/exams_test.go
package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	account "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	documents "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	labs "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type documentStub struct {
	documents.Service
	document documents.ExamDocumentOutput
	denied   bool
}

type examFileStorageStub struct {
	uri       string
	expiresIn time.Duration
}

func (s *examFileStorageStub) Upload(context.Context, io.Reader, string, string) (string, error) {
	return "", nil
}

func (s *examFileStorageStub) Delete(context.Context, string) error { return nil }

func (s *examFileStorageStub) GetSignedURL(_ context.Context, uri string, expiresIn time.Duration) (string, error) {
	s.uri = uri
	s.expiresIn = expiresIn
	return "https://storage.test/signed", nil
}

func (s *documentStub) FindByID(context.Context, *account.Account, uuid.UUID) (*documents.ExamDocumentOutput, error) {
	if s.denied {
		return nil, apperr.Forbidden("Sem acesso.")
	}
	return &s.document, nil
}
func (s *documentStub) ListByPatient(context.Context, *account.Account, uuid.UUID, int, int) ([]documents.ExamDocumentOutput, error) {
	if s.denied {
		return nil, apperr.Forbidden("Sem acesso.")
	}
	return []documents.ExamDocumentOutput{s.document}, nil
}

type reviewStub struct {
	created, confirmed, deleted bool
	path                        string
	err                         error
	denied                      bool
}

func (s *reviewStub) Create(_ context.Context, _ *account.Account, in documents.CreateDraftInput) (*documents.ExamDocumentOutput, error) {
	if s.denied {
		return nil, apperr.Forbidden("Sem acesso.")
	}
	s.created = true
	s.path = in.LocalPath
	state := "pending"
	return &documents.ExamDocumentOutput{ID: uuid.New(), PatientID: in.PatientID, ReviewStatus: &state}, s.err
}
func (s *reviewStub) Extraction(context.Context, *account.Account, uuid.UUID) (*extraction.Result, error) {
	if s.denied {
		return nil, apperr.Forbidden("Sem acesso.")
	}
	return &extraction.Result{SummaryText: "Glicose: 90"}, s.err
}
func (s *reviewStub) Confirm(context.Context, *account.Account, uuid.UUID) (*labs.LabReportOutput, error) {
	if s.denied {
		return nil, apperr.Forbidden("Sem acesso.")
	}
	s.confirmed = true
	return &labs.LabReportOutput{ID: uuid.New()}, s.err
}
func (s *reviewStub) Delete(context.Context, *account.Account, uuid.UUID) error {
	if s.denied {
		return apperr.Forbidden("Sem acesso.")
	}
	s.deleted = true
	return s.err
}

func reviewRouter(denied bool, review *reviewStub) (*gin.Engine, uuid.UUID, uuid.UUID) {
	gin.SetMode(gin.TestMode)
	doc, patient := uuid.New(), uuid.New()
	review.denied = denied
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), &account.Account{ID: uuid.New()}))
	})
	NewExams(&documentStub{document: documents.ExamDocumentOutput{ID: doc, PatientID: patient}, denied: denied}, review, review, nil).RegisterHumaRoutes(humagin.New(r, huma.DefaultConfig("test", "test")), nil)
	return r, doc, patient
}

func TestReviewRoutesRequirePatientAccess(t *testing.T) {
	for _, suffix := range []string{"/extraction", "/confirmation", ""} {
		method := http.MethodGet
		if suffix == "/confirmation" {
			method = http.MethodPost
		}
		if suffix == "" {
			method = http.MethodDelete
		}
		stub := &reviewStub{}
		router, id, _ := reviewRouter(true, stub)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(method, "/exam-documents/"+id.String()+suffix, nil))
		if res.Code != 403 || stub.confirmed || stub.deleted {
			t.Fatalf("access bypass: %d", res.Code)
		}
	}
}
func TestUploadCreatesDraftAndCleansStandaloneFile(t *testing.T) {
	for _, denied := range []bool{false, true} {
		stub := &reviewStub{}
		router, _, patient := reviewRouter(denied, stub)
		body, contentType := standaloneLabMultipart(t, "lab.pdf", "application/pdf", []byte("%PDF-1.4"))
		req := httptest.NewRequest(http.MethodPost, "/patients/"+patient.String()+"/exam-documents", body)
		req.Header.Set("Content-Type", contentType)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if denied {
			if res.Code != 403 || stub.created {
				t.Fatal("unauthorized upload")
			}
			continue
		}
		if res.Code != 201 || !bytes.Contains(res.Body.Bytes(), []byte(`"review_status":"pending"`)) {
			t.Fatalf("%d: %s", res.Code, res.Body)
		}
		if _, err := os.Stat(stub.path); !os.IsNotExist(err) {
			t.Fatal("standalone file leaked")
		}
	}
}

func TestUploadRejectsPDFOverPersistentLimit(t *testing.T) {
	stub := &reviewStub{}
	router, _, patient := reviewRouter(false, stub)
	payload := append([]byte("%PDF-"), make([]byte, examDocumentMaxFileSize)...)
	body, contentType := standaloneLabMultipart(t, "large.pdf", "application/pdf", payload)
	req := httptest.NewRequest(http.MethodPost, "/patients/"+patient.String()+"/exam-documents", body)
	req.Header.Set("Content-Type", contentType)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusRequestEntityTooLarge || stub.created {
		t.Fatalf("expected 413 without draft creation, got %d: %s", res.Code, res.Body.String())
	}
}

func TestGetExamDocumentFileUsesPersistentLifetime(t *testing.T) {
	documentID := uuid.New()
	storageURI := "supabase://exam-documents/patients/patient/document.pdf"
	storage := &examFileStorageStub{}
	handler := NewExams(&documentStub{document: documents.ExamDocumentOutput{
		ID:         documentID,
		StorageURI: storageURI,
	}}, nil, nil, storage)
	ctx := helpers.ContextWithCurrentAccount(t.Context(), &account.Account{ID: uuid.New()})
	startedAt := time.Now().UTC()

	output, err := handler.getExamDocumentFile(ctx, &examDocumentInput{DocumentID: documentID})

	if err != nil {
		t.Fatalf("get exam document file: %v", err)
	}
	if storage.uri != storageURI || storage.expiresIn != examDocumentFileURLLifetime {
		t.Fatalf("signed URL request = (%q, %s), want (%q, %s)", storage.uri, storage.expiresIn, storageURI, examDocumentFileURLLifetime)
	}
	if output.Body.URL != "https://storage.test/signed" {
		t.Fatalf("signed URL = %q", output.Body.URL)
	}
	minimumExpiration := startedAt.Add(examDocumentFileURLLifetime)
	maximumExpiration := time.Now().UTC().Add(examDocumentFileURLLifetime)
	if output.Body.ExpiresAt.Before(minimumExpiration) || output.Body.ExpiresAt.After(maximumExpiration) {
		t.Fatalf("expiration = %s, want between %s and %s", output.Body.ExpiresAt, minimumExpiration, maximumExpiration)
	}
}

func TestReviewSuccessAndConflict(t *testing.T) {
	for _, failure := range []bool{false, true} {
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			stub := &reviewStub{}
			if failure {
				stub.err = apperr.Conflict("Documento confirmado.")
			}
			router, id, _ := reviewRouter(false, stub)
			url := "/exam-documents/" + id.String()
			if method == http.MethodPost {
				url += "/confirmation"
			}
			res := httptest.NewRecorder()
			router.ServeHTTP(res, httptest.NewRequest(method, url, nil))
			want := 200
			if method == http.MethodDelete {
				want = 204
			}
			if failure {
				want = 409
			}
			if res.Code != want {
				t.Fatalf("%s: %d %s", method, res.Code, res.Body)
			}
		}
	}
}
