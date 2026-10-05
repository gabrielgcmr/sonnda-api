// internal/features/patient/problem/domain/problem_test.go
package problemdomain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	testPatientID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testActorID   = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	testTime      = time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
)

func newTestProblem(t *testing.T, classification Classification) Problem {
	t.Helper()
	problem, _, err := NewProblem(NewProblemParams{
		PatientID: testPatientID, ActorAccountID: testActorID,
		Name: " Hipertensão ", Classification: classification, OccurredAt: testTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	return problem
}

func changeTestProblem(p Problem, action Action, after Snapshot, reason string, sources ...uuid.UUID) (Problem, HistoryEvent, error) {
	return p.Change(ChangeParams{
		Action: action, After: after, ActorAccountID: testActorID,
		OccurredAt: p.UpdatedAt.Add(time.Second), Reason: reason, SourceProblemIDs: sources,
	})
}

func TestNewProblemRequiresExplicitClassification(t *testing.T) {
	for _, classification := range []Classification{"", "latent", "ACUTE"} {
		_, _, err := NewProblem(NewProblemParams{
			PatientID: testPatientID, ActorAccountID: testActorID,
			Name: "Problema", Classification: classification, OccurredAt: testTime,
		})
		if !errors.Is(err, ErrInvalidClassification) {
			t.Fatalf("classification %q: expected ErrInvalidClassification, got %v", classification, err)
		}
	}
	for _, classification := range []Classification{ClassificationAcute, ClassificationChronic} {
		problem := newTestProblem(t, classification)
		if problem.State.Classification != classification || problem.State.ClinicalStatus != ClinicalStatusActive || problem.State.AdministrativeStatus != AdministrativeStatusValid {
			t.Fatalf("unexpected initial state: %+v", problem.State)
		}
	}
}

func TestNewProblemPreservesCodingAndCreationAudit(t *testing.T) {
	coding := &CID11{Code: " CA23 ", System: " ICD-11 ", Version: " 2026 "}
	problem, event, err := NewProblem(NewProblemParams{
		PatientID: testPatientID, ActorAccountID: testActorID,
		Name: " Gripe ", CID11: coding, Classification: ClassificationAcute, OccurredAt: testTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	coding.Code = "modified"
	if problem.State.Name != "Gripe" || problem.State.CID11.Code != "CA23" || problem.State.CID11.System != "ICD-11" || problem.State.CID11.Version != "2026" {
		t.Fatalf("coding or name was not preserved: %+v", problem.State)
	}
	if problem.Version != 1 || event.Version != 1 || event.Action != ActionCreated || event.Before != nil || event.After.CID11.Code != "CA23" || event.ActorAccountID != testActorID || event.PatientID != testPatientID {
		t.Fatalf("invalid creation event: %+v", event)
	}
	problem.State.CID11.Code = "changed again"
	if event.After.CID11.Code != "CA23" {
		t.Fatal("audit snapshot aliases the live problem")
	}
}

func TestProblemRejectsChronicResolvedEvenOnDirectStateValidation(t *testing.T) {
	problem := newTestProblem(t, ClassificationChronic)
	problem.State.ClinicalStatus = ClinicalStatusResolved
	if err := problem.Validate(); !errors.Is(err, ErrChronicResolved) {
		t.Fatalf("expected ErrChronicResolved, got %v", err)
	}
}

func TestResolveAndReopenProduceVersionedAudit(t *testing.T) {
	problem := newTestProblem(t, ClassificationAcute)
	after := problem.State
	after.ClinicalStatus = ClinicalStatusResolved
	resolved, event, err := changeTestProblem(problem, ActionResolved, after, "")
	if err != nil {
		t.Fatal(err)
	}
	if problem.State.ClinicalStatus != ClinicalStatusActive || resolved.Version != 2 || event.Version != 2 || event.Before.ClinicalStatus != ClinicalStatusActive || event.After.ClinicalStatus != ClinicalStatusResolved {
		t.Fatalf("invalid resolution result: %+v %+v", resolved, event)
	}
	reopenedState := resolved.State
	reopenedState.ClinicalStatus = ClinicalStatusActive
	reopened, reopenEvent, err := changeTestProblem(resolved, ActionReopened, reopenedState, "")
	if err != nil || reopened.Version != 3 || reopenEvent.Before.ClinicalStatus != ClinicalStatusResolved {
		t.Fatalf("invalid reopen result: %+v %+v %v", reopened, reopenEvent, err)
	}
}

func TestEditAndClassificationKeepAuditAndRejectChronicResolved(t *testing.T) {
	problem := newTestProblem(t, ClassificationAcute)
	edited := problem.State
	edited.Name = "Novo nome"
	edited.CID11 = &CID11{Code: "A01", System: "ICD-11", Version: "2026"}
	updated, editEvent, err := changeTestProblem(problem, ActionEdited, edited, "")
	if err != nil || editEvent.Before.Name != "Hipertensão" || editEvent.After.Name != "Novo nome" {
		t.Fatalf("invalid edit event: %+v %v", editEvent, err)
	}
	classified := updated.State
	classified.Classification = ClassificationChronic
	chronic, classifyEvent, err := changeTestProblem(updated, ActionClassified, classified, "")
	if err != nil || chronic.Version != 3 || classifyEvent.Before.Classification != ClassificationAcute || classifyEvent.After.Classification != ClassificationChronic {
		t.Fatalf("invalid classification event: %+v %v", classifyEvent, err)
	}
	resolved := problem.State
	resolved.ClinicalStatus = ClinicalStatusResolved
	resolvedProblem, _, err := changeTestProblem(problem, ActionResolved, resolved, "")
	if err != nil {
		t.Fatal(err)
	}
	invalid := resolvedProblem.State
	invalid.Classification = ClassificationChronic
	if _, _, err := changeTestProblem(resolvedProblem, ActionClassified, invalid, ""); !errors.Is(err, ErrChronicResolved) {
		t.Fatalf("expected ErrChronicResolved, got %v", err)
	}
}

func TestTransitionRulesRejectInvalidChanges(t *testing.T) {
	acute := newTestProblem(t, ClassificationAcute)
	chronic := newTestProblem(t, ClassificationChronic)
	chronicResolved := chronic.State
	chronicResolved.ClinicalStatus = ClinicalStatusResolved
	wrongEdit := acute.State
	wrongEdit.ClinicalStatus = ClinicalStatusResolved
	cases := []struct {
		name    string
		problem Problem
		action  Action
		after   Snapshot
		want    error
	}{
		{"chronic resolution", chronic, ActionResolved, chronicResolved, ErrChronicResolved},
		{"reopen active", acute, ActionReopened, acute.State, ErrInvalidTransition},
		{"edit clinical status", acute, ActionEdited, wrongEdit, ErrInvalidTransition},
		{"edit without change", acute, ActionEdited, acute.State, ErrInvalidTransition},
		{"classify without change", acute, ActionClassified, acute.State, ErrInvalidTransition},
		{"resolve and classify together", acute, ActionResolved, Snapshot{Name: acute.State.Name, Classification: ClassificationChronic, ClinicalStatus: ClinicalStatusResolved, AdministrativeStatus: AdministrativeStatusValid}, ErrChronicResolved},
		{"unknown action", acute, Action("unknown"), acute.State, ErrInvalidTransition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := changeTestProblem(tc.problem, tc.action, tc.after, "")
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestRectificationAndMergeStates(t *testing.T) {
	problem := newTestProblem(t, ClassificationAcute)
	invalid := problem.State
	invalid.AdministrativeStatus = AdministrativeStatusEnteredInError
	if _, _, err := changeTestProblem(problem, ActionRectified, invalid, "  "); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("expected reason requirement, got %v", err)
	}
	rectified, event, err := changeTestProblem(problem, ActionRectified, invalid, "  registro indevido  ")
	if err != nil || event.Reason != "registro indevido" {
		t.Fatalf("invalid rectification: %+v %v", event, err)
	}
	if _, _, err := changeTestProblem(rectified, ActionEdited, rectified.State, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected terminal administrative state, got %v", err)
	}
	destination := uuid.New()
	merged := problem.State
	merged.AdministrativeStatus = AdministrativeStatusMerged
	merged.MergedIntoID = &destination
	mergedProblem, _, err := changeTestProblem(problem, ActionMergedSource, merged, "")
	if err != nil || mergedProblem.State.MergedIntoID == nil || *mergedProblem.State.MergedIntoID != destination {
		t.Fatalf("invalid merged source: %+v %v", mergedProblem, err)
	}
	if _, _, err := changeTestProblem(mergedProblem, ActionResolved, mergedProblem.State, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected merged source to be terminal, got %v", err)
	}
}

func TestMergeDestinationRecordsSources(t *testing.T) {
	problem := newTestProblem(t, ClassificationAcute)
	source := uuid.New()
	after := problem.State
	after.Name = "Nome final"
	next, event, err := changeTestProblem(problem, ActionMergedDestination, after, "", source)
	if err != nil || next.Version != 2 || len(event.SourceProblemIDs) != 1 || event.SourceProblemIDs[0] != source {
		t.Fatalf("invalid merge destination event: %+v %v", event, err)
	}
	for _, sources := range [][]uuid.UUID{{}, {problem.ID}, {source, source}, {uuid.Nil}} {
		if _, _, err := changeTestProblem(problem, ActionMergedDestination, after, "", sources...); !errors.Is(err, ErrInvalidMergeSources) {
			t.Fatalf("sources %v: expected ErrInvalidMergeSources, got %v", sources, err)
		}
	}
}
