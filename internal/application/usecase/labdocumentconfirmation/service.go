// internal/application/usecase/labdocumentconfirmation/service.go
package labdocumentconfirmation

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	documents "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	domain "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	laboratory "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory"
	labs "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type Repository interface {
	Confirm(context.Context, uuid.UUID, uuid.UUID, *labs.LabReport, string, *string) (uuid.UUID, error)
	GetExtraction(context.Context, uuid.UUID) ([]byte, error)
}
type Service struct {
	authorizer documents.Authorizer
	repository Repository
	labs       laboratory.Repository
}

func New(authorizer documents.Authorizer, repo Repository, labs laboratory.Repository) *Service {
	return &Service{authorizer: authorizer, repository: repo, labs: labs}
}

func (s *Service) Confirm(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*laboratory.LabReportOutput, error) {
	document, err := s.authorizer.AuthorizeDocument(ctx, currentAccount, id, documents.ConfirmDocument)
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, apperr.NotFound("Documento não encontrado.")
	}
	if document.ReviewStatus == nil || (*document.ReviewStatus != "pending" && *document.ReviewStatus != "confirmed") {
		return nil, apperr.Conflict("Este documento não pode ser confirmado.")
	}
	if *document.ReviewStatus == "confirmed" && document.LabReportID != nil {
		return s.report(ctx, *document.LabReportID)
	}
	snapshot, err := s.repository.GetExtraction(ctx, id)
	if err != nil {
		return nil, apperr.Internal("Falha ao ler a extração armazenada.", err)
	}
	result, err := documents.DecodeExtractionSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	if !extraction.Usable(&result.Report) {
		return nil, apperr.DomainRuleViolation("A extração não contém resultados utilizáveis.")
	}
	report, err := mapExtractedToDomain(document.PatientID, document.UploadedByUserID, &result.Report)
	if err != nil {
		return nil, apperr.DomainRuleViolation("Os dados extraídos não permitem confirmar este exame.")
	}
	report.ExamDocumentID = &id
	reportID, err := s.repository.Confirm(ctx, id, currentAccount.ID, report, generateLabFingerprint(document.PatientID, report), result.Report.RawText)
	if errors.Is(err, domain.ErrReviewConflict) {
		return nil, apperr.Conflict("O rascunho foi descartado ou não pode ser confirmado.")
	}
	if errors.Is(err, labs.ErrLabReportAlreadyExists) {
		return nil, apperr.AlreadyExists("Este exame já está cadastrado em outro documento.")
	}
	if err != nil {
		return nil, apperr.Internal("Falha ao confirmar o exame.", err)
	}
	return s.report(ctx, reportID)
}

func (s *Service) report(ctx context.Context, id uuid.UUID) (*laboratory.LabReportOutput, error) {
	report, err := s.labs.FindByID(ctx, id)
	if err != nil {
		return nil, apperr.Internal("Falha ao consultar o exame confirmado.", err)
	}
	if report == nil {
		return nil, apperr.NotFound("Exame não encontrado.")
	}
	return toOutput(report), nil
}
