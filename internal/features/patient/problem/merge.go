// internal/features/patient/problem/merge.go
package problem

import (
	"context"
	"time"

	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type MergeSourceInput struct {
	ID      uuid.UUID
	Version int64
}

type MergeInput struct {
	Version        int64
	Sources        []MergeSourceInput
	Name           string
	CID11          *problemdomain.CID11
	CID11Set       bool
	Classification problemdomain.Classification
	ClinicalStatus problemdomain.ClinicalStatus
}

// Merge consolidates valid source problems into one valid destination. Every
// problem and its audit event are saved atomically with version checks.
func (s *Service) Merge(ctx context.Context, actorID, patientID, destinationID uuid.UUID, input MergeInput) (problemdomain.Problem, error) {
	if err := s.authorize(ctx, actorID, patientID, MergeProblems); err != nil {
		return problemdomain.Problem{}, err
	}
	if input.Version < 1 {
		return problemdomain.Problem{}, apperr.DomainRuleViolation("versão deve ser maior que zero")
	}
	if !input.CID11Set {
		return problemdomain.Problem{}, apperr.DomainRuleViolation("cid11 deve ser informado como objeto ou null")
	}
	if err := validateMergeInputs(destinationID, input.Sources); err != nil {
		return problemdomain.Problem{}, err
	}

	destination, err := s.repo.Get(ctx, patientID, destinationID)
	if err != nil {
		return problemdomain.Problem{}, storeError(err)
	}
	if destination.Version != input.Version {
		return problemdomain.Problem{}, storeError(ErrVersionConflict)
	}

	sources := make([]problemdomain.Problem, 0, len(input.Sources))
	for _, sourceInput := range input.Sources {
		source, getErr := s.repo.Get(ctx, patientID, sourceInput.ID)
		if getErr != nil {
			return problemdomain.Problem{}, storeError(getErr)
		}
		if source.Version != sourceInput.Version {
			return problemdomain.Problem{}, storeError(ErrVersionConflict)
		}
		sources = append(sources, source)
	}

	when := time.Now().UTC()
	sourceIDs := make([]uuid.UUID, len(sources))
	for i := range sources {
		sourceIDs[i] = sources[i].ID
	}
	finalState := destination.State
	finalState.Name = input.Name
	finalState.CID11 = input.CID11
	finalState.Classification = input.Classification
	finalState.ClinicalStatus = input.ClinicalStatus
	finalState.AdministrativeStatus = problemdomain.AdministrativeStatusValid
	finalState.MergedIntoID = nil
	mergedDestination, destinationEvent, err := destination.Change(problemdomain.ChangeParams{
		Action: problemdomain.ActionMergedDestination, After: finalState,
		ActorAccountID: actorID, OccurredAt: when, SourceProblemIDs: sourceIDs,
	})
	if err != nil {
		return problemdomain.Problem{}, changeError(err)
	}

	updates := make([]VersionedUpdate, 0, len(sources)+1)
	updates = append(updates, VersionedUpdate{ExpectedVersion: destination.Version, Problem: mergedDestination, Event: destinationEvent})
	for _, source := range sources {
		after := source.State
		after.AdministrativeStatus = problemdomain.AdministrativeStatusMerged
		after.MergedIntoID = &destinationID
		mergedSource, sourceEvent, changeErr := source.Change(problemdomain.ChangeParams{
			Action: problemdomain.ActionMergedSource, After: after,
			ActorAccountID: actorID, OccurredAt: when,
		})
		if changeErr != nil {
			return problemdomain.Problem{}, changeError(changeErr)
		}
		updates = append(updates, VersionedUpdate{ExpectedVersion: source.Version, Problem: mergedSource, Event: sourceEvent})
	}
	if err := s.repo.Merge(ctx, updates); err != nil {
		return problemdomain.Problem{}, storeError(err)
	}
	return mergedDestination, nil
}

func validateMergeInputs(destinationID uuid.UUID, sources []MergeSourceInput) error {
	if destinationID == uuid.Nil || len(sources) == 0 {
		return apperr.DomainRuleViolation("informe ao menos uma origem válida")
	}
	seen := make(map[uuid.UUID]struct{}, len(sources))
	for _, source := range sources {
		if source.ID == uuid.Nil || source.ID == destinationID || source.Version < 1 {
			return apperr.DomainRuleViolation("origens da unificação são inválidas")
		}
		if _, exists := seen[source.ID]; exists {
			return apperr.DomainRuleViolation("origens da unificação não podem se repetir")
		}
		seen[source.ID] = struct{}{}
	}
	return nil
}
