// internal/features/patient/problem/postgres/write.go
package postgres

import (
	"context"
	"sort"
	"time"

	problemrepository "github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	problemsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/problem"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) Create(ctx context.Context, problem problemdomain.Problem, event problemdomain.HistoryEvent) error {
	if err := validateCreatePair(problem, event); err != nil {
		return err
	}
	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return persistenceError("begin patient problem creation", err)
	}
	defer rollback(ctx, tx)

	queries := r.queries.WithTx(tx)
	if err := queries.CreatePatientProblem(ctx, problemParams(problem)); err != nil {
		return persistenceError("create patient problem", err)
	}
	if err := createHistory(ctx, queries, event); err != nil {
		return err
	}
	return persistenceError("commit patient problem creation", tx.Commit(ctx))
}

func (r *Repository) Update(ctx context.Context, expectedVersion int64, problem problemdomain.Problem, event problemdomain.HistoryEvent) error {
	if err := validateUpdatePair(expectedVersion, problem, event); err != nil {
		return err
	}
	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return persistenceError("begin patient problem update", err)
	}
	defer rollback(ctx, tx)

	queries := r.queries.WithTx(tx)
	rows, err := queries.UpdatePatientProblemVersion(ctx, updateParams(expectedVersion, problem))
	if err != nil {
		return persistenceError("update patient problem", err)
	}
	if rows != 1 {
		return problemrepository.ErrVersionConflict
	}
	if err := createHistory(ctx, queries, event); err != nil {
		return err
	}
	return persistenceError("commit patient problem update", tx.Commit(ctx))
}

// Merge applies every destination/source update and audit event in one
// transaction. A stale version rolls the entire operation back.
func (r *Repository) Merge(ctx context.Context, updates []problemrepository.VersionedUpdate) error {
	if err := validateMergeUpdates(updates); err != nil {
		return err
	}
	ordered := append([]problemrepository.VersionedUpdate(nil), updates...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Problem.ID.String() < ordered[j].Problem.ID.String()
	})
	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return persistenceError("begin patient problem merge", err)
	}
	defer rollback(ctx, tx)

	queries := r.queries.WithTx(tx)
	for _, update := range ordered {
		rows, updateErr := queries.UpdatePatientProblemVersion(ctx, updateParams(update.ExpectedVersion, update.Problem))
		if updateErr != nil {
			return persistenceError("merge patient problem", updateErr)
		}
		if rows != 1 {
			return problemrepository.ErrVersionConflict
		}
		if historyErr := createHistory(ctx, queries, update.Event); historyErr != nil {
			return historyErr
		}
	}
	return persistenceError("commit patient problem merge", tx.Commit(ctx))
}

func validateMergeUpdates(updates []problemrepository.VersionedUpdate) error {
	if len(updates) < 2 {
		return problemrepository.ErrInconsistentAuditEvent
	}
	patientID := updates[0].Problem.PatientID
	seen := make(map[uuid.UUID]struct{}, len(updates))
	destinations, sources := 0, 0
	var destinationID, actorID uuid.UUID
	var occurredAt time.Time
	declaredSources := make(map[uuid.UUID]struct{}, len(updates)-1)
	for _, update := range updates {
		if update.Problem.PatientID != patientID {
			return problemrepository.ErrInconsistentAuditEvent
		}
		if _, exists := seen[update.Problem.ID]; exists {
			return problemrepository.ErrInconsistentAuditEvent
		}
		seen[update.Problem.ID] = struct{}{}
		if err := validateUpdatePair(update.ExpectedVersion, update.Problem, update.Event); err != nil {
			return err
		}
		switch update.Event.Action {
		case problemdomain.ActionMergedDestination:
			destinations++
			destinationID = update.Problem.ID
			actorID = update.Event.ActorAccountID
			occurredAt = update.Event.OccurredAt
			if update.Problem.State.AdministrativeStatus != problemdomain.AdministrativeStatusValid ||
				update.Problem.State.MergedIntoID != nil || len(update.Event.SourceProblemIDs) != len(updates)-1 {
				return problemrepository.ErrInconsistentAuditEvent
			}
			for _, sourceID := range update.Event.SourceProblemIDs {
				if sourceID == uuid.Nil {
					return problemrepository.ErrInconsistentAuditEvent
				}
				if _, exists := declaredSources[sourceID]; exists {
					return problemrepository.ErrInconsistentAuditEvent
				}
				declaredSources[sourceID] = struct{}{}
			}
		case problemdomain.ActionMergedSource:
			sources++
			if update.Problem.State.AdministrativeStatus != problemdomain.AdministrativeStatusMerged ||
				update.Problem.State.MergedIntoID == nil || len(update.Event.SourceProblemIDs) != 0 {
				return problemrepository.ErrInconsistentAuditEvent
			}
		default:
			return problemrepository.ErrInconsistentAuditEvent
		}
	}
	if destinations != 1 || sources != len(updates)-1 {
		return problemrepository.ErrInconsistentAuditEvent
	}
	for _, update := range updates {
		if update.Event.Action != problemdomain.ActionMergedSource {
			continue
		}
		if *update.Problem.State.MergedIntoID != destinationID || update.Event.ActorAccountID != actorID ||
			!update.Event.OccurredAt.Equal(occurredAt) {
			return problemrepository.ErrInconsistentAuditEvent
		}
		if _, exists := declaredSources[update.Problem.ID]; !exists {
			return problemrepository.ErrInconsistentAuditEvent
		}
	}
	return nil
}

func createHistory(ctx context.Context, queries *problemsqlc.Queries, event problemdomain.HistoryEvent) error {
	params, err := historyParams(event)
	if err != nil {
		return persistenceError("encode patient problem history", err)
	}
	if err := queries.CreatePatientProblemHistory(ctx, params); err != nil {
		return persistenceError("create patient problem history", err)
	}
	return nil
}

func rollback(ctx context.Context, tx pgx.Tx) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(cleanupCtx)
}

func validateCreatePair(problem problemdomain.Problem, event problemdomain.HistoryEvent) error {
	if err := problem.Validate(); err != nil {
		return err
	}
	if !validEventIdentity(problem, event) || problem.Version != 1 ||
		event.Version != 1 || event.Action != problemdomain.ActionCreated ||
		event.Before != nil || event.Reason != "" || len(event.SourceProblemIDs) != 0 ||
		event.ActorAccountID != problem.CreatedByAccountID ||
		!event.OccurredAt.Equal(problem.CreatedAt) || !problem.UpdatedAt.Equal(problem.CreatedAt) ||
		!snapshotsEqual(event.After, problem.State) {
		return problemrepository.ErrInconsistentAuditEvent
	}
	return nil
}

func validateUpdatePair(expectedVersion int64, problem problemdomain.Problem, event problemdomain.HistoryEvent) error {
	if err := problem.Validate(); err != nil {
		return err
	}
	if expectedVersion < 1 || problem.Version != expectedVersion+1 ||
		!validEventIdentity(problem, event) || event.Version != problem.Version ||
		event.Action == problemdomain.ActionCreated || event.Before == nil ||
		!event.OccurredAt.Equal(problem.UpdatedAt) || !snapshotsEqual(event.After, problem.State) {
		return problemrepository.ErrInconsistentAuditEvent
	}
	if err := event.Before.Validate(problem.ID); err != nil {
		return problemrepository.ErrInconsistentAuditEvent
	}
	return nil
}

func validEventIdentity(problem problemdomain.Problem, event problemdomain.HistoryEvent) bool {
	return event.ID != uuid.Nil && event.ProblemID == problem.ID &&
		event.PatientID == problem.PatientID && event.ActorAccountID != uuid.Nil &&
		!event.OccurredAt.IsZero()
}
