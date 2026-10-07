// internal/features/patient/problem/change.go
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

type EditInput struct {
	Version int64
	Name    string
	CID11   *problemdomain.CID11
}

type ClassifyInput struct {
	Version        int64
	Classification problemdomain.Classification
}

type RectifyInput struct {
	Version int64
	Reason  string
}

// Edit replaces the details. A nil CID11 explicitly removes the previous coding.
func (s *Service) Edit(ctx context.Context, actorID, patientID, problemID uuid.UUID, input EditInput) (problemdomain.Problem, error) {
	return s.change(ctx, actorID, patientID, problemID, input.Version, authz.EditProblem, problemdomain.ActionEdited, "", func(state problemdomain.Snapshot) problemdomain.Snapshot {
		state.Name, state.CID11 = input.Name, input.CID11
		return state
	})
}

func (s *Service) Classify(ctx context.Context, actorID, patientID, problemID uuid.UUID, input ClassifyInput) (problemdomain.Problem, error) {
	return s.change(ctx, actorID, patientID, problemID, input.Version, authz.ClassifyProblem, problemdomain.ActionClassified, "", func(state problemdomain.Snapshot) problemdomain.Snapshot {
		state.Classification = input.Classification
		return state
	})
}

func (s *Service) Resolve(ctx context.Context, actorID, patientID, problemID uuid.UUID, version int64) (problemdomain.Problem, error) {
	return s.change(ctx, actorID, patientID, problemID, version, authz.ResolveProblem, problemdomain.ActionResolved, "", func(state problemdomain.Snapshot) problemdomain.Snapshot {
		state.ClinicalStatus = problemdomain.ClinicalStatusResolved
		return state
	})
}

func (s *Service) Reopen(ctx context.Context, actorID, patientID, problemID uuid.UUID, version int64) (problemdomain.Problem, error) {
	return s.change(ctx, actorID, patientID, problemID, version, authz.ReopenProblem, problemdomain.ActionReopened, "", func(state problemdomain.Snapshot) problemdomain.Snapshot {
		state.ClinicalStatus = problemdomain.ClinicalStatusActive
		return state
	})
}

func (s *Service) Rectify(ctx context.Context, actorID, patientID, problemID uuid.UUID, input RectifyInput) (problemdomain.Problem, error) {
	return s.change(ctx, actorID, patientID, problemID, input.Version, authz.RectifyProblem, problemdomain.ActionRectified, input.Reason, func(state problemdomain.Snapshot) problemdomain.Snapshot {
		state.AdministrativeStatus = problemdomain.AdministrativeStatusEnteredInError
		return state
	})
}

func (s *Service) change(ctx context.Context, actorID, patientID, problemID uuid.UUID, version int64, permission authz.ProblemAction, action problemdomain.Action, reason string, apply func(problemdomain.Snapshot) problemdomain.Snapshot) (problemdomain.Problem, error) {
	if err := s.authorize(ctx, actorID, patientID, permission); err != nil {
		return problemdomain.Problem{}, err
	}
	if version < 1 {
		return problemdomain.Problem{}, apperr.DomainRuleViolation("versão deve ser maior que zero")
	}
	p, err := s.repo.Get(ctx, patientID, problemID)
	if err != nil {
		return problemdomain.Problem{}, storeError(err)
	}
	if p.Version != version {
		return problemdomain.Problem{}, storeError(ErrVersionConflict)
	}
	next, event, err := p.Change(problemdomain.ChangeParams{
		Action: action, After: apply(p.State), ActorAccountID: actorID,
		OccurredAt: time.Now().UTC(), Reason: reason,
	})
	if err != nil {
		return problemdomain.Problem{}, changeError(err)
	}
	// The atomic compare-and-swap also rejects changes between Get and Update,
	// including a concurrent classification change while resolving an acute problem.
	if err := s.repo.Update(ctx, version, next, event); err != nil {
		return problemdomain.Problem{}, storeError(err)
	}
	return next, nil
}

func changeError(err error) error {
	switch {
	case errors.Is(err, problemdomain.ErrInvalidName):
		return apperr.DomainRuleViolation("nome do problema obrigatório")
	case errors.Is(err, problemdomain.ErrInvalidCID11):
		return apperr.DomainRuleViolation("CID-11 deve informar code, system e version")
	case errors.Is(err, problemdomain.ErrInvalidClassification):
		return apperr.DomainRuleViolation("classificação deve ser acute ou chronic")
	case errors.Is(err, problemdomain.ErrChronicResolved):
		return apperr.DomainRuleViolation("problema crônico não pode estar resolvido")
	case errors.Is(err, problemdomain.ErrReasonRequired):
		return apperr.DomainRuleViolation("motivo da retificação é obrigatório")
	case errors.Is(err, problemdomain.ErrInvalidMergeSources), errors.Is(err, problemdomain.ErrInvalidMergeTarget):
		return apperr.DomainRuleViolation("origens ou destino da unificação são inválidos")
	case errors.Is(err, problemdomain.ErrInvalidTransition):
		return apperr.DomainRuleViolation("alteração incompatível com o estado atual do problema ou sem mudanças")
	default:
		return apperr.Internal("falha ao alterar problema", err)
	}
}
