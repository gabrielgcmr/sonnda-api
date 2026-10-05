// internal/features/patient/problem/service.go
package problem

import (
	"context"
	"errors"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type Service struct {
	repo       Repository
	authorizer authz.ProblemAuthorizer
}

func New(repo Repository, authorizer authz.ProblemAuthorizer) *Service {
	return &Service{repo: repo, authorizer: authorizer}
}

type CreateInput struct {
	Name           string
	CID11          *problemdomain.CID11
	Classification problemdomain.Classification
}

type Page[T any] struct {
	Items   []T
	Limit   int
	Offset  int
	HasMore bool
}

func (s *Service) Create(ctx context.Context, actorID, patientID uuid.UUID, input CreateInput) (problemdomain.Problem, error) {
	if err := s.authorize(ctx, actorID, patientID, authz.CreateProblem); err != nil {
		return problemdomain.Problem{}, err
	}
	p, event, err := problemdomain.NewProblem(problemdomain.NewProblemParams{
		PatientID: patientID, ActorAccountID: actorID, Name: input.Name,
		CID11: input.CID11, Classification: input.Classification, OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		switch {
		case errors.Is(err, problemdomain.ErrInvalidName):
			return p, apperr.DomainRuleViolation("nome do problema obrigatório")
		case errors.Is(err, problemdomain.ErrInvalidClassification):
			return p, apperr.DomainRuleViolation("classificação deve ser acute ou chronic")
		case errors.Is(err, problemdomain.ErrInvalidCID11):
			return p, apperr.DomainRuleViolation("CID-11 deve informar code, system e version")
		default:
			return p, apperr.Internal("falha ao criar problema", err)
		}
	}
	if err := s.repo.Create(ctx, p, event); err != nil {
		return problemdomain.Problem{}, storeError(err)
	}
	return p, nil
}

func (s *Service) Get(ctx context.Context, actorID, patientID, problemID uuid.UUID) (problemdomain.Problem, error) {
	if err := s.authorize(ctx, actorID, patientID, authz.ReadProblem); err != nil {
		return problemdomain.Problem{}, err
	}
	p, err := s.repo.Get(ctx, patientID, problemID)
	return p, storeError(err)
}

func (s *Service) List(ctx context.Context, actorID, patientID uuid.UUID, filter ListFilter) (Page[problemdomain.Problem], error) {
	if err := s.authorize(ctx, actorID, patientID, authz.ListProblems); err != nil {
		return Page[problemdomain.Problem]{}, err
	}
	page, err := normalizePagination(filter.Pagination)
	if err != nil {
		return Page[problemdomain.Problem]{}, err
	}
	if filter.ClinicalStatus == "" {
		filter.ClinicalStatus = "all"
	}
	if filter.AdministrativeStatus == "" {
		filter.AdministrativeStatus = "valid"
	}
	if filter.ClinicalStatus != "all" && filter.ClinicalStatus != "active" && filter.ClinicalStatus != "resolved" {
		return Page[problemdomain.Problem]{}, apperr.DomainRuleViolation("situação clínica inválida")
	}
	switch filter.AdministrativeStatus {
	case "all", "valid", "merged", "entered_in_error":
	default:
		return Page[problemdomain.Problem]{}, apperr.DomainRuleViolation("condição administrativa inválida")
	}
	filter.Pagination = Pagination{Limit: page.Limit + 1, Offset: page.Offset}
	items, err := s.repo.List(ctx, patientID, filter)
	if err != nil {
		return Page[problemdomain.Problem]{}, storeError(err)
	}
	return makePage(items, page), nil
}

func (s *Service) History(ctx context.Context, actorID, patientID, problemID uuid.UUID, pagination Pagination) (Page[problemdomain.HistoryEvent], error) {
	if err := s.authorize(ctx, actorID, patientID, authz.ReadHistory); err != nil {
		return Page[problemdomain.HistoryEvent]{}, err
	}
	page, err := normalizePagination(pagination)
	if err != nil {
		return Page[problemdomain.HistoryEvent]{}, err
	}
	// A missing or cross-patient problem is a 404, even for an empty history page.
	if _, err := s.repo.Get(ctx, patientID, problemID); err != nil {
		return Page[problemdomain.HistoryEvent]{}, storeError(err)
	}
	items, err := s.repo.ListHistory(ctx, patientID, problemID, Pagination{Limit: page.Limit + 1, Offset: page.Offset})
	if err != nil {
		return Page[problemdomain.HistoryEvent]{}, storeError(err)
	}
	return makePage(items, page), nil
}

func (s *Service) authorize(ctx context.Context, actorID, patientID uuid.UUID, action authz.ProblemAction) error {
	if s == nil || s.repo == nil || s.authorizer == nil {
		return apperr.Internal("serviço indisponível", errors.New("problem service dependencies not configured"))
	}
	return s.authorizer.Authorize(ctx, actorID, patientID, action)
}

func normalizePagination(page Pagination) (Pagination, error) {
	if page.Limit == 0 {
		page.Limit = 20
	}
	if page.Limit < 1 || page.Limit > 100 || page.Offset < 0 || page.Offset > 2147483647 {
		return page, apperr.DomainRuleViolation("paginação inválida")
	}
	return page, nil
}

func makePage[T any](items []T, page Pagination) Page[T] {
	hasMore := len(items) > page.Limit
	if hasMore {
		items = items[:page.Limit]
	}
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Limit: page.Limit, Offset: page.Offset, HasMore: hasMore}
}

func storeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return apperr.NotFound("problema não encontrado")
	}
	return &apperr.AppError{Kind: apperr.INFRA_DATABASE_ERROR, Message: "falha técnica", Cause: err}
}
