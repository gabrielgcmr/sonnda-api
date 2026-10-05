// internal/features/patient/problem/postgres/write.go
package postgres

import (
	"context"
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
