// internal/features/patient/problem/postgres/rectify_integration_test.go
//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	problemservice "github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
)

func TestRepositoryPersistsRectificationAndReasonAtomically(t *testing.T) {
	_, repository, patientID, actorID := newProblemTestDatabase(t)
	ctx := context.Background()
	patientProblem, created := testProblem(t, patientID, actorID, "Registro indevido")
	if err := repository.Create(ctx, patientProblem, created); err != nil {
		t.Fatal(err)
	}

	after := patientProblem.State
	after.AdministrativeStatus = problemdomain.AdministrativeStatusEnteredInError
	rectified, event, err := patientProblem.Change(problemdomain.ChangeParams{
		Action:         problemdomain.ActionRectified,
		After:          after,
		ActorAccountID: actorID,
		OccurredAt:     patientProblem.UpdatedAt.Add(time.Second),
		Reason:         "  registro criado para o paciente errado  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(ctx, patientProblem.Version, rectified, event); err != nil {
		t.Fatal(err)
	}

	stored, err := repository.Get(ctx, patientID, patientProblem.ID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := repository.ListHistory(ctx, patientID, patientProblem.ID, problemservice.Pagination{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	valid, err := repository.List(ctx, patientID, problemservice.ListFilter{
		Pagination:           problemservice.Pagination{Limit: 10},
		ClinicalStatus:       "all",
		AdministrativeStatus: "valid",
	})
	if err != nil {
		t.Fatal(err)
	}
	rectifiedItems, err := repository.List(ctx, patientID, problemservice.ListFilter{
		Pagination:           problemservice.Pagination{Limit: 10},
		ClinicalStatus:       "all",
		AdministrativeStatus: "entered_in_error",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.State.AdministrativeStatus != problemdomain.AdministrativeStatusEnteredInError ||
		stored.Version != 2 || len(events) != 2 || events[0].Action != problemdomain.ActionRectified ||
		events[0].Reason != "registro criado para o paciente errado" ||
		events[0].Before == nil || events[0].Before.AdministrativeStatus != problemdomain.AdministrativeStatusValid ||
		events[0].After.AdministrativeStatus != problemdomain.AdministrativeStatusEnteredInError ||
		len(valid) != 0 || len(rectifiedItems) != 1 || rectifiedItems[0].ID != patientProblem.ID {
		t.Fatalf("invalid persisted rectification: problem=%+v events=%+v valid=%+v rectified=%+v", stored, events, valid, rectifiedItems)
	}
}
