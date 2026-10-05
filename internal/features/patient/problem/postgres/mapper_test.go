// internal/features/patient/problem/postgres/mapper_test.go
package postgres

import (
	"encoding/json"
	"testing"
	"time"

	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
)

func TestHistoryParamsUsesStableSnapshotShape(t *testing.T) {
	when := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.FixedZone("local", -3*60*60))
	event := problemdomain.HistoryEvent{
		ID: uuid.New(), ProblemID: uuid.New(), PatientID: uuid.New(), Version: 1,
		Action: problemdomain.ActionCreated, ActorAccountID: uuid.New(), OccurredAt: when,
		After: problemdomain.Snapshot{
			Name: "Problema", CID11: &problemdomain.CID11{Code: "CA23", System: "ICD-11", Version: "2026"},
			Classification:       problemdomain.ClassificationAcute,
			ClinicalStatus:       problemdomain.ClinicalStatusActive,
			AdministrativeStatus: problemdomain.AdministrativeStatusValid,
		},
	}
	params, err := historyParams(event)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(params.AfterSnapshot), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["name"] != "Problema" || snapshot["classification"] != "acute" || snapshot["clinical_status"] != "active" {
		t.Fatalf("unexpected snapshot: %s", params.AfterSnapshot)
	}
	cid, ok := snapshot["cid11"].(map[string]any)
	if !ok || cid["code"] != "CA23" || cid["system"] != "ICD-11" || cid["version"] != "2026" {
		t.Fatalf("unexpected CID-11 snapshot: %s", params.AfterSnapshot)
	}
	if params.BeforeSnapshot.Valid || params.Reason.Valid || !params.OccurredAt.Time.Equal(when.UTC()) {
		t.Fatalf("unexpected nullable/time mapping: %+v", params)
	}
}

func TestPersistencePairValidation(t *testing.T) {
	when := time.Date(2026, time.October, 5, 15, 0, 0, 0, time.UTC)
	problem, created, err := problemdomain.NewProblem(problemdomain.NewProblemParams{
		PatientID: uuid.New(), ActorAccountID: uuid.New(), Name: "Problema",
		Classification: problemdomain.ClassificationAcute, OccurredAt: when,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCreatePair(problem, created); err != nil {
		t.Fatalf("valid create pair rejected: %v", err)
	}
	badCreated := created
	badCreated.Version = 2
	if err := validateCreatePair(problem, badCreated); err == nil {
		t.Fatal("inconsistent create pair accepted")
	}
	after := problem.State
	after.ClinicalStatus = problemdomain.ClinicalStatusResolved
	updated, changed, err := problem.Change(problemdomain.ChangeParams{
		Action: problemdomain.ActionResolved, After: after,
		ActorAccountID: uuid.New(), OccurredAt: when.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateUpdatePair(1, updated, changed); err != nil {
		t.Fatalf("valid update pair rejected: %v", err)
	}
	if err := validateUpdatePair(2, updated, changed); err == nil {
		t.Fatal("stale expected version accepted")
	}
}
