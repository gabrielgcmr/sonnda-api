// internal/features/documentprocessing/drafts_test.go
package documentprocessing

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	domain "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	"github.com/google/uuid"
)

type allowDocumentAuthorizer struct{}

func (allowDocumentAuthorizer) AuthorizeStandalone(*accountdomain.Account, Action) error { return nil }
func (allowDocumentAuthorizer) AuthorizePatient(context.Context, *accountdomain.Account, uuid.UUID, Action) error {
	return nil
}
func (allowDocumentAuthorizer) AuthorizeDocument(_ context.Context, _ *accountdomain.Account, id uuid.UUID, _ Action) (*domain.ExamDocument, error) {
	return &domain.ExamDocument{ID: id, PatientID: uuid.New()}, nil
}

func documentTestAccount() *accountdomain.Account {
	return &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}
}

type draftStoreStub struct {
	snapshot  []byte
	createErr error
	deleting  bool
	finished  bool
}

func (s *draftStoreStub) CreateDraft(_ context.Context, _ *domain.ExamDocument, data []byte) error {
	s.snapshot = data
	return s.createErr
}
func (s *draftStoreStub) GetExtraction(context.Context, uuid.UUID) ([]byte, error) {
	return s.snapshot, nil
}
func (s *draftStoreStub) BeginDelete(context.Context, uuid.UUID) (*domain.ExamDocument, error) {
	s.deleting = true
	return &domain.ExamDocument{StorageURI: "supabase://exam-documents/test/file.pdf"}, nil
}
func (s *draftStoreStub) FinishDelete(context.Context, uuid.UUID) error {
	s.finished = true
	return nil
}

type pdfStub struct {
	result *labreport.Result
	err    error
}

func (s pdfStub) ExtractPDF(context.Context, string, string) (*labreport.Result, error) {
	return s.result, s.err
}

type storageStub struct {
	uploaded, deleted bool
	deleteErr         error
}

func (s *storageStub) Upload(context.Context, io.Reader, string, string) (string, error) {
	s.uploaded = true
	return "supabase://exam-documents/test/file.pdf", nil
}
func (s *storageStub) Delete(context.Context, string) error { s.deleted = true; return s.deleteErr }
func (s *storageStub) GetSignedURL(context.Context, string, time.Duration) (string, error) {
	return "https://test/file.pdf", nil
}
func draftResult() *labreport.Result {
	value := "Negativo"
	return &labreport.Result{Status: labreport.ExtractionStatusPartial, SummaryText: "Resultado: Negativo", Report: labreport.ExtractedLabReport{Tests: []labreport.ExtractedTestResult{{TestName: "Teste", Items: []labreport.ExtractedTestItem{{ParameterName: "Resultado", ResultValue: &value}}}}}}
}
func TestDraftCreationAndCompensation(t *testing.T) {
	for _, failure := range []bool{false, true} {
		repo, storage := &draftStoreStub{}, &storageStub{}
		if failure {
			repo.createErr = errors.New("database unavailable")
		}
		service := NewDrafts(repo, pdfStub{result: draftResult()}, storage, allowDocumentAuthorizer{})
		path := filepath.Join(t.TempDir(), "exam.pdf")
		if err := os.WriteFile(path, []byte("%PDF-1.4"), 0600); err != nil {
			t.Fatal(err)
		}
		account := documentTestAccount()
		doc, err := service.Create(context.Background(), account, CreateDraftInput{PatientID: uuid.New(), LocalPath: path, Filename: "exam.pdf"})
		if failure {
			if err == nil || !storage.deleted {
				t.Fatal("uploaded PDF not compensated")
			}
			continue
		}
		if err != nil || doc.ReviewStatus == nil || *doc.ReviewStatus != "pending" || !storage.uploaded {
			t.Fatalf("bad creation: %+v %v", doc, err)
		}
		result, err := service.Extraction(context.Background(), account, doc.ID)
		if err != nil || result.SummaryText != draftResult().SummaryText {
			t.Fatal("snapshot changed")
		}
	}
}
func TestUnusableExtractionDoesNotUpload(t *testing.T) {
	storage := &storageStub{}
	repo := &draftStoreStub{}
	service := NewDrafts(repo, pdfStub{result: &labreport.Result{}}, storage, allowDocumentAuthorizer{})
	if _, err := service.Create(context.Background(), documentTestAccount(), CreateDraftInput{PatientID: uuid.New()}); err == nil || storage.uploaded || repo.snapshot != nil {
		t.Fatal("unusable extraction persisted")
	}
}
func TestDeleteCanResumeAfterStorageFailure(t *testing.T) {
	repo := &draftStoreStub{}
	storage := &storageStub{deleteErr: errors.New("storage unavailable")}
	service := NewDrafts(repo, nil, storage, allowDocumentAuthorizer{})
	id := uuid.New()
	account := documentTestAccount()
	if err := service.Delete(context.Background(), account, id); err == nil || repo.finished || !repo.deleting {
		t.Fatal("lost reference before removing PDF")
	}
	storage.deleteErr = nil
	if err := service.Delete(context.Background(), account, id); err != nil || !repo.finished {
		t.Fatal("delete did not resume")
	}
}
