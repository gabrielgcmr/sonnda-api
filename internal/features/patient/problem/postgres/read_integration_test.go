// internal/features/patient/problem/postgres/read_integration_test.go
package postgres

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
)

func TestRepositoryReadSnapshotsAndPatientIsolation(t *testing.T) {
	_, repo, patientID, actorID := newProblemTestDatabase(t)
	ctx := context.Background()
	p, created, err := problemdomain.NewProblem(problemdomain.NewProblemParams{
		PatientID: patientID, ActorAccountID: actorID, Name: "Teste",
		CID11:          &problemdomain.CID11{Code: "BA00", System: "ICD-11", Version: "2026"},
		Classification: problemdomain.ClassificationAcute, OccurredAt: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, p, created); err != nil {
		t.Fatal(err)
	}
	after := p.State
	after.Name = "Nome corrigido"
	after.CID11 = nil
	next, changed, err := p.Change(problemdomain.ChangeParams{Action: problemdomain.ActionEdited, After: after, ActorAccountID: actorID, OccurredAt: p.UpdatedAt.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, p.Version, next, changed); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, patientID, p.ID)
	if err != nil || !reflect.DeepEqual(got, next) {
		t.Fatalf("problem roundtrip: got=%+v want=%+v err=%v", got, next, err)
	}
	events, err := repo.ListHistory(ctx, patientID, p.ID, problem.Pagination{Limit: 10})
	if err != nil || len(events) != 2 {
		t.Fatalf("history: %+v, %v", events, err)
	}
	if events[0].Version != 2 || events[1].Version != 1 || events[1].Before != nil || !reflect.DeepEqual(events[0].Before, changed.Before) || !reflect.DeepEqual(events[0].After, changed.After) || !reflect.DeepEqual(events[1].After, created.After) || events[0].ActorAccountID != actorID || !events[0].OccurredAt.Equal(changed.OccurredAt) {
		t.Fatalf("snapshot roundtrip: %+v", events)
	}
	events, err = repo.ListHistory(ctx, patientID, p.ID, problem.Pagination{Limit: 1, Offset: 1})
	if err != nil || len(events) != 1 || events[0].ID != created.ID {
		t.Fatalf("history pagination: %+v %v", events, err)
	}
	otherPatient := uuid.New()
	if _, err := repo.Get(ctx, otherPatient, p.ID); !errors.Is(err, problem.ErrNotFound) {
		t.Fatalf("cross-patient detail: %v", err)
	}
	events, err = repo.ListHistory(ctx, otherPatient, p.ID, problem.Pagination{Limit: 10})
	if err != nil || len(events) != 0 {
		t.Fatalf("cross-patient history leaked: %+v %v", events, err)
	}
	items, err := repo.List(ctx, otherPatient, problem.ListFilter{Pagination: problem.Pagination{Limit: 10}, ClinicalStatus: "all", AdministrativeStatus: "all"})
	if err != nil || len(items) != 0 {
		t.Fatalf("cross-patient list leaked: %+v %v", items, err)
	}
}

func TestRepositoryListFiltersAndStablePagination(t *testing.T) {
	_, repo, patientID, actorID := newProblemTestDatabase(t)
	ctx := context.Background()
	when := time.Now().UTC().Truncate(time.Microsecond)
	var validIDs []string
	for index := 0; index < 4; index++ {
		p, created, err := problemdomain.NewProblem(problemdomain.NewProblemParams{PatientID: patientID, ActorAccountID: actorID, Name: "Repetido", Classification: problemdomain.ClassificationAcute, OccurredAt: when})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Create(ctx, p, created); err != nil {
			t.Fatal(err)
		}
		if index < 2 {
			validIDs = append(validIDs, p.ID.String())
			continue
		}
		after := p.State
		action, reason := problemdomain.ActionResolved, ""
		if index == 2 {
			after.ClinicalStatus = problemdomain.ClinicalStatusResolved
		} else {
			after.AdministrativeStatus = problemdomain.AdministrativeStatusEnteredInError
			action, reason = problemdomain.ActionRectified, "Registro incorreto"
		}
		next, event, err := p.Change(problemdomain.ChangeParams{Action: action, After: after, ActorAccountID: actorID, OccurredAt: when.Add(time.Second), Reason: reason})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Update(ctx, p.Version, next, event); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		clinical, administrative string
		count                    int
	}{
		{"all", "all", 4}, {"all", "valid", 3}, {"active", "valid", 2}, {"resolved", "valid", 1}, {"all", "entered_in_error", 1}, {"all", "merged", 0},
	} {
		items, err := repo.List(ctx, patientID, problem.ListFilter{Pagination: problem.Pagination{Limit: 10}, ClinicalStatus: test.clinical, AdministrativeStatus: test.administrative})
		if err != nil || len(items) != test.count {
			t.Fatalf("filter %+v: count=%d err=%v", test, len(items), err)
		}
		if test.administrative == "entered_in_error" {
			events, err := repo.ListHistory(ctx, patientID, items[0].ID, problem.Pagination{Limit: 1})
			if err != nil || len(events) != 1 || events[0].Reason != "Registro incorreto" {
				t.Fatalf("rectification reason: %+v %v", events, err)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(validIDs)))
	for offset, id := range validIDs {
		items, err := repo.List(ctx, patientID, problem.ListFilter{Pagination: problem.Pagination{Limit: 1, Offset: offset}, ClinicalStatus: "active", AdministrativeStatus: "valid"})
		if err != nil || len(items) != 1 || items[0].ID.String() != id {
			t.Fatalf("tied timestamp page %d: %+v %v", offset, items, err)
		}
	}
}
