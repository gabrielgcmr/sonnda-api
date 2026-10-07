// internal/features/patient/problem/postgres/merge_integration_test.go
package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	problemrepository "github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
)

func buildMergeUpdates(t *testing.T, destination problemdomain.Problem, sources []problemdomain.Problem, actorID uuid.UUID, when time.Time) []problemrepository.VersionedUpdate {
	t.Helper()
	sourceIDs := make([]uuid.UUID, len(sources))
	for i, source := range sources {
		sourceIDs[i] = source.ID
	}
	finalState := destination.State
	finalState.Name = "Problema consolidado"
	finalState.CID11 = nil
	mergedDestination, destinationEvent, err := destination.Change(problemdomain.ChangeParams{
		Action: problemdomain.ActionMergedDestination, After: finalState,
		ActorAccountID: actorID, OccurredAt: when, SourceProblemIDs: sourceIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	updates := []problemrepository.VersionedUpdate{{ExpectedVersion: destination.Version, Problem: mergedDestination, Event: destinationEvent}}
	for _, source := range sources {
		after := source.State
		after.AdministrativeStatus = problemdomain.AdministrativeStatusMerged
		after.MergedIntoID = &destination.ID
		mergedSource, sourceEvent, changeErr := source.Change(problemdomain.ChangeParams{
			Action: problemdomain.ActionMergedSource, After: after,
			ActorAccountID: actorID, OccurredAt: when,
		})
		if changeErr != nil {
			t.Fatal(changeErr)
		}
		updates = append(updates, problemrepository.VersionedUpdate{ExpectedVersion: source.Version, Problem: mergedSource, Event: sourceEvent})
	}
	return updates
}

func createMergeProblems(t *testing.T, repository *Repository, patientID, actorID uuid.UUID) []problemdomain.Problem {
	t.Helper()
	problems := make([]problemdomain.Problem, 3)
	for i, name := range []string{"Destino", "Origem um", "Origem dois"} {
		p, event := testProblem(t, patientID, actorID, name)
		if err := repository.Create(context.Background(), p, event); err != nil {
			t.Fatal(err)
		}
		problems[i] = p
	}
	return problems
}

func TestRepositoryMergePersistsDestinationSourcesAndAuditAtomically(t *testing.T) {
	client, repository, patientID, actorID := newProblemTestDatabase(t)
	ctx := context.Background()
	problems := createMergeProblems(t, repository, patientID, actorID)
	when := time.Now().UTC().Add(time.Second)
	updates := buildMergeUpdates(t, problems[0], problems[1:], actorID, when)
	if err := repository.Merge(ctx, updates); err != nil {
		t.Fatal(err)
	}

	var destinationName, destinationStatus string
	var destinationVersion int64
	if err := client.Pool().QueryRow(ctx, `SELECT name, administrative_status, version FROM patient_problems WHERE id=$1`, problems[0].ID).
		Scan(&destinationName, &destinationStatus, &destinationVersion); err != nil {
		t.Fatal(err)
	}
	if destinationName != "Problema consolidado" || destinationStatus != "valid" || destinationVersion != 2 {
		t.Fatalf("destination name=%q status=%s version=%d", destinationName, destinationStatus, destinationVersion)
	}
	for _, source := range problems[1:] {
		var status string
		var target uuid.UUID
		var version int64
		if err := client.Pool().QueryRow(ctx, `SELECT administrative_status, merged_into_id, version FROM patient_problems WHERE id=$1`, source.ID).
			Scan(&status, &target, &version); err != nil {
			t.Fatal(err)
		}
		if status != "merged" || target != problems[0].ID || version != 2 {
			t.Fatalf("source=%s status=%s target=%s version=%d", source.ID, status, target, version)
		}
	}
	var action string
	var sourceIDs []uuid.UUID
	if err := client.Pool().QueryRow(ctx, `SELECT action, source_problem_ids FROM patient_problem_history WHERE problem_id=$1 AND version=2`, problems[0].ID).
		Scan(&action, &sourceIDs); err != nil {
		t.Fatal(err)
	}
	if action != "merged_destination" || len(sourceIDs) != 2 || sourceIDs[0] != problems[1].ID || sourceIDs[1] != problems[2].ID {
		t.Fatalf("destination audit action=%s sources=%v", action, sourceIDs)
	}
	var totalHistory int
	if err := client.Pool().QueryRow(ctx, `SELECT count(*) FROM patient_problem_history WHERE problem_id = ANY($1)`, []uuid.UUID{problems[0].ID, problems[1].ID, problems[2].ID}).Scan(&totalHistory); err != nil {
		t.Fatal(err)
	}
	if totalHistory != 6 {
		t.Fatalf("history events=%d", totalHistory)
	}
}

func TestRepositoryMergeRollsBackAllUpdatesOnStaleSource(t *testing.T) {
	client, repository, patientID, actorID := newProblemTestDatabase(t)
	ctx := context.Background()
	problems := createMergeProblems(t, repository, patientID, actorID)
	updates := buildMergeUpdates(t, problems[0], problems[1:], actorID, time.Now().UTC().Add(2*time.Second))

	after := problems[2].State
	after.Name = "Alterado concorrentemente"
	concurrent, event, err := problems[2].Change(problemdomain.ChangeParams{
		Action: problemdomain.ActionEdited, After: after,
		ActorAccountID: actorID, OccurredAt: problems[2].UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(ctx, 1, concurrent, event); err != nil {
		t.Fatal(err)
	}
	if err := repository.Merge(ctx, updates); !errors.Is(err, problemrepository.ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}

	for i, p := range problems {
		var status string
		var version int64
		if err := client.Pool().QueryRow(ctx, `SELECT administrative_status, version FROM patient_problems WHERE id=$1`, p.ID).Scan(&status, &version); err != nil {
			t.Fatal(err)
		}
		wantVersion := int64(1)
		if i == 2 {
			wantVersion = 2
		}
		if status != "valid" || version != wantVersion {
			t.Fatalf("problem=%s status=%s version=%d want=%d", p.ID, status, version, wantVersion)
		}
	}
	var historyCount int
	if err := client.Pool().QueryRow(ctx, `SELECT count(*) FROM patient_problem_history WHERE problem_id = ANY($1)`, []uuid.UUID{problems[0].ID, problems[1].ID, problems[2].ID}).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != 4 {
		t.Fatalf("merge left partial audit: count=%d", historyCount)
	}
}
