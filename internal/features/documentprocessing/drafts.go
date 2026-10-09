// internal/features/documentprocessing/drafts.go
package documentprocessing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	documents "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type DraftRepository interface {
	CreateDraft(context.Context, *documents.ExamDocument, []byte) error
	GetExtraction(context.Context, uuid.UUID) ([]byte, error)
	BeginDelete(context.Context, uuid.UUID) (*documents.ExamDocument, error)
	FinishDelete(context.Context, uuid.UUID) error
}

type PDFExtractor interface {
	ExtractPDF(context.Context, string, string) (*extraction.Result, error)
}

type Drafts struct {
	repo       DraftRepository
	extractor  PDFExtractor
	storage    FileStorageService
	authorizer Authorizer
}

type CreateDraftInput struct {
	PatientID           uuid.UUID
	LocalPath, Filename string
}

func NewDrafts(repo DraftRepository, extractor PDFExtractor, storage FileStorageService, authorizer Authorizer) *Drafts {
	return &Drafts{repo: repo, extractor: extractor, storage: storage, authorizer: authorizer}
}

func (s *Drafts) Create(ctx context.Context, currentAccount *accountdomain.Account, input CreateDraftInput) (*ExamDocumentOutput, error) {
	if input.PatientID == uuid.Nil {
		return nil, apperr.Validation("Paciente é obrigatório.")
	}
	if err := s.authorizer.AuthorizePatient(ctx, currentAccount, input.PatientID, UploadDocument); err != nil {
		return nil, err
	}
	result, err := s.extractor.ExtractPDF(ctx, input.LocalPath, input.Filename)
	if err != nil {
		return nil, err
	}
	if result == nil || !extraction.Usable(&result.Report) {
		return nil, apperr.DomainRuleViolation("O PDF não contém resultados laboratoriais utilizáveis.")
	}
	snapshot, err := EncodeExtractionSnapshot(result)
	if err != nil {
		return nil, apperr.Internal("Falha ao preparar a extração.", err)
	}
	file, err := os.Open(input.LocalPath)
	if err != nil {
		return nil, apperr.Internal("Falha ao abrir o PDF.", err)
	}
	defer file.Close()
	uri, err := s.storage.Upload(ctx, file, fmt.Sprintf("patients/%s/exam-documents/%s.pdf", input.PatientID, uuid.NewString()), "application/pdf")
	if err != nil {
		return nil, apperr.Internal("Falha ao armazenar o PDF.", err)
	}
	document, err := documents.NewExamDocument(input.PatientID, currentAccount.ID, uri, input.Filename, "application/pdf")
	if err == nil {
		status, kind := "pending", documents.ExamTypeLaboratory
		method := "pdf_text_raw"
		document.ReviewStatus, document.ExamType, document.ExtractionMethod = &status, &kind, &method
		document.Status = documents.DocumentStatusProcessed
		err = s.repo.CreateDraft(ctx, document, snapshot)
	}
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		cleanupErr := s.storage.Delete(cleanup, uri)
		return nil, apperr.Internal("Falha ao salvar o rascunho.", errors.Join(err, cleanupErr))
	}
	return mapDomainDocumentToOutput(document), nil
}

func (s *Drafts) Extraction(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*extraction.Result, error) {
	document, err := s.authorizer.AuthorizeDocument(ctx, currentAccount, id, ReadExtraction)
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, apperr.NotFound("Documento não encontrado.")
	}
	data, err := s.repo.GetExtraction(ctx, id)
	if err != nil {
		return nil, draftError(err)
	}
	if len(data) == 0 {
		return nil, apperr.NotFound("Este documento não possui uma extração para conferência.")
	}
	result, err := DecodeExtractionSnapshot(data)
	if err != nil {
		return nil, apperr.Internal("Falha ao ler a extração armazenada.", err)
	}
	return result, nil
}

func (s *Drafts) Delete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error {
	document, err := s.authorizer.AuthorizeDocument(ctx, currentAccount, id, DiscardDocument)
	if err != nil {
		return err
	}
	if document == nil {
		return apperr.NotFound("Documento não encontrado.")
	}
	doc, err := s.repo.BeginDelete(ctx, id)
	if err != nil {
		return draftError(err)
	}
	if doc == nil {
		return nil
	}
	if err := s.storage.Delete(ctx, doc.StorageURI); err != nil {
		return apperr.Internal("Não foi possível excluir o PDF. Tente descartar novamente.", err)
	}
	return draftError(s.repo.FinishDelete(ctx, id))
}

func draftError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, documents.ErrReviewConflict) {
		return apperr.Conflict("Somente rascunhos pendentes podem ser descartados ou confirmados.")
	}
	return apperr.Internal("Falha ao salvar o documento.", err)
}
