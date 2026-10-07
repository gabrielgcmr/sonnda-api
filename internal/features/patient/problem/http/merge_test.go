// internal/features/patient/problem/http/merge_test.go
package problemhttp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
)

func mergeTestStore(t *testing.T) (*testStore, []problemdomain.Problem) {
	t.Helper()
	patientID, creatorID := uuid.New(), uuid.New()
	problems := make([]problemdomain.Problem, 3)
	events := make([]problemdomain.HistoryEvent, 3)
	for i, name := range []string{"Destino", "Origem um", "Origem dois"} {
		p, event, err := problemdomain.NewProblem(problemdomain.NewProblemParams{
			PatientID: patientID, ActorAccountID: creatorID, Name: name,
			Classification: problemdomain.ClassificationAcute,
			OccurredAt:     time.Now().UTC().Add(-time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		problems[i], events[i] = p, event
	}
	byID := make(map[uuid.UUID]problemdomain.Problem, len(problems))
	for _, p := range problems {
		byID[p.ID] = p
	}
	return &testStore{p: problems[0], problems: byID, events: events}, problems
}

func mergePath(problems []problemdomain.Problem) string {
	return fmt.Sprintf("/patients/%s/problems/%s/merge", problems[0].PatientID, problems[0].ID)
}

func mergeBody(problems []problemdomain.Problem, cid string) string {
	return fmt.Sprintf(`{"version":%d,"sources":[{"id":"%s","version":%d},{"id":"%s","version":%d}],"name":"  Consolidado  ","cid11":%s,"classification":"chronic","clinical_status":"active"}`,
		problems[0].Version, problems[1].ID, problems[1].Version, problems[2].ID, problems[2].Version, cid)
}

func TestMergeRequiresProfessionalWithAccessAndRecordsAllChanges(t *testing.T) {
	for _, kind := range []accountdomain.AccountType{accountdomain.AccountTypeBasicCare, accountdomain.AccountTypeProfessional} {
		for _, access := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/access=%t", kind, access), func(t *testing.T) {
				store, problems := mergeTestStore(t)
				router, actorID := testRouter(store, kind, access)
				response := request(router, "POST", mergePath(problems), mergeBody(problems, `{"code":" BA00 ","system":"ICD-11","version":"2026"}`))
				if !access || kind != accountdomain.AccountTypeProfessional {
					if response.Code != 403 || store.calls != 0 || len(store.events) != 3 {
						t.Fatalf("status=%d calls=%d events=%d body=%s", response.Code, store.calls, len(store.events), response.Body.String())
					}
					return
				}
				var result ProblemResponse
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if response.Code != 200 || result.ID != problems[0].ID || result.Version != 2 || result.Name != "Consolidado" ||
					result.CID11 == nil || result.CID11.Code != "BA00" || result.Classification != "chronic" || result.ClinicalStatus != "active" || result.AdministrativeStatus != "valid" {
					t.Fatalf("status=%d result=%+v body=%s", response.Code, result, response.Body.String())
				}
				for _, source := range problems[1:] {
					current := store.problems[source.ID]
					if current.Version != 2 || current.State.AdministrativeStatus != problemdomain.AdministrativeStatusMerged || current.State.MergedIntoID == nil || *current.State.MergedIntoID != problems[0].ID || current.State.Name != source.State.Name {
						t.Fatalf("invalid merged source: %+v", current)
					}
				}
				if len(store.events) != 6 {
					t.Fatalf("events=%d", len(store.events))
				}
				destinationEvent := store.events[3]
				if destinationEvent.Action != problemdomain.ActionMergedDestination || destinationEvent.ActorAccountID != actorID || len(destinationEvent.SourceProblemIDs) != 2 || destinationEvent.SourceProblemIDs[0] != problems[1].ID || destinationEvent.SourceProblemIDs[1] != problems[2].ID {
					t.Fatalf("invalid destination audit: %+v", destinationEvent)
				}
				for _, event := range store.events[4:] {
					if event.Action != problemdomain.ActionMergedSource || event.ActorAccountID != actorID || len(event.SourceProblemIDs) != 0 {
						t.Fatalf("invalid source audit: %+v", event)
					}
				}
			})
		}
	}
}

func TestMergeRequiresExplicitCIDAndAcceptsNull(t *testing.T) {
	store, problems := mergeTestStore(t)
	router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
	body := strings.Replace(mergeBody(problems, "null"), `,"cid11":null`, "", 1)
	response := request(router, "POST", mergePath(problems), body)
	if response.Code != 422 || len(store.events) != 3 {
		t.Fatalf("omitted CID: %d %s", response.Code, response.Body.String())
	}
	response = request(router, "POST", mergePath(problems), mergeBody(problems, "null"))
	if response.Code != 200 || store.p.State.CID11 != nil || len(store.events) != 6 {
		t.Fatalf("explicit null CID: %d %s", response.Code, response.Body.String())
	}
}

func TestMergeRejectsInvalidSelectionsWithoutPartialChanges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func([]problemdomain.Problem) string
		want  int
	}{
		{"no sources", func(_ []problemdomain.Problem) string {
			return `{"version":1,"sources":[],"name":"Final","cid11":null,"classification":"acute","clinical_status":"active"}`
		}, 422},
		{"self source", func(p []problemdomain.Problem) string {
			return fmt.Sprintf(`{"version":1,"sources":[{"id":"%s","version":1}],"name":"Final","cid11":null,"classification":"acute","clinical_status":"active"}`, p[0].ID)
		}, 422},
		{"duplicate source", func(p []problemdomain.Problem) string {
			return fmt.Sprintf(`{"version":1,"sources":[{"id":"%s","version":1},{"id":"%s","version":1}],"name":"Final","cid11":null,"classification":"acute","clinical_status":"active"}`, p[1].ID, p[1].ID)
		}, 422},
		{"chronic resolved", func(p []problemdomain.Problem) string {
			return strings.Replace(mergeBody(p, "null"), `"clinical_status":"active"`, `"clinical_status":"resolved"`, 1)
		}, 422},
		{"stale destination", func(p []problemdomain.Problem) string {
			return strings.Replace(mergeBody(p, "null"), `"version":1`, `"version":2`, 1)
		}, 409},
		{"stale source", func(p []problemdomain.Problem) string {
			return strings.Replace(mergeBody(p, "null"), fmt.Sprintf(`"id":"%s","version":1`, p[1].ID), fmt.Sprintf(`"id":"%s","version":2`, p[1].ID), 1)
		}, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, problems := mergeTestStore(t)
			router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
			response := request(router, "POST", mergePath(problems), tc.build(problems))
			if response.Code != tc.want || len(store.events) != 3 {
				t.Fatalf("status=%d want=%d events=%d body=%s", response.Code, tc.want, len(store.events), response.Body.String())
			}
			for _, original := range problems {
				if current := store.problems[original.ID]; current.Version != 1 || current.State.AdministrativeStatus != problemdomain.AdministrativeStatusValid {
					t.Fatalf("partial update: %+v", current)
				}
			}
		})
	}
}

func TestMergeRejectsCrossPatientTerminalAndRepeatedSources(t *testing.T) {
	for _, scenario := range []string{"cross patient", "terminal destination", "terminal source"} {
		t.Run(scenario, func(t *testing.T) {
			store, problems := mergeTestStore(t)
			switch scenario {
			case "cross patient":
				source := store.problems[problems[1].ID]
				source.PatientID = uuid.New()
				store.problems[source.ID] = source
			case "terminal destination":
				destination := store.problems[problems[0].ID]
				destination.State.AdministrativeStatus = problemdomain.AdministrativeStatusEnteredInError
				store.problems[destination.ID], store.p = destination, destination
			case "terminal source":
				source := store.problems[problems[1].ID]
				source.State.AdministrativeStatus = problemdomain.AdministrativeStatusEnteredInError
				store.problems[source.ID] = source
			}
			router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
			response := request(router, "POST", mergePath(problems), mergeBody(problems, "null"))
			want := 422
			if scenario == "cross patient" {
				want = 404
			}
			if response.Code != want || len(store.events) != 3 {
				t.Fatalf("status=%d want=%d events=%d body=%s", response.Code, want, len(store.events), response.Body.String())
			}
		})
	}

	store, problems := mergeTestStore(t)
	router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
	body := mergeBody(problems, "null")
	if response := request(router, "POST", mergePath(problems), body); response.Code != 200 {
		t.Fatalf("first merge: %d %s", response.Code, response.Body.String())
	}
	if response := request(router, "POST", mergePath(problems), body); response.Code != 409 || len(store.events) != 6 {
		t.Fatalf("repeated merge: %d %s events=%d", response.Code, response.Body.String(), len(store.events))
	}
}

func TestSuccessiveMergePreservesRecoverableOriginChain(t *testing.T) {
	store, problems := mergeTestStore(t)
	router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
	if response := request(router, "POST", mergePath(problems), mergeBody(problems, "null")); response.Code != 200 {
		t.Fatalf("first merge: %d %s", response.Code, response.Body.String())
	}
	firstDestinationEvent := store.events[3]

	finalDestination, created, err := problemdomain.NewProblem(problemdomain.NewProblemParams{
		PatientID: problems[0].PatientID, ActorAccountID: uuid.New(), Name: "Destino final",
		Classification: problemdomain.ClassificationAcute, OccurredAt: time.Now().UTC().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	store.problems[finalDestination.ID] = finalDestination
	store.events = append(store.events, created)
	path := fmt.Sprintf("/patients/%s/problems/%s/merge", finalDestination.PatientID, finalDestination.ID)
	body := fmt.Sprintf(`{"version":1,"sources":[{"id":"%s","version":2}],"name":"Final","cid11":null,"classification":"acute","clinical_status":"active"}`, problems[0].ID)
	if response := request(router, "POST", path, body); response.Code != 200 {
		t.Fatalf("successive merge: %d %s", response.Code, response.Body.String())
	}

	intermediate := store.problems[problems[0].ID]
	if intermediate.State.AdministrativeStatus != problemdomain.AdministrativeStatusMerged || intermediate.State.MergedIntoID == nil || *intermediate.State.MergedIntoID != finalDestination.ID {
		t.Fatalf("intermediate destination was not preserved as an origin: %+v", intermediate)
	}
	if firstDestinationEvent.ProblemID != problems[0].ID || len(firstDestinationEvent.SourceProblemIDs) != 2 || firstDestinationEvent.SourceProblemIDs[0] != problems[1].ID || firstDestinationEvent.SourceProblemIDs[1] != problems[2].ID {
		t.Fatalf("lost original ancestry: %+v", firstDestinationEvent)
	}
	lastDestinationEvent := store.events[len(store.events)-2]
	if lastDestinationEvent.ProblemID != finalDestination.ID || len(lastDestinationEvent.SourceProblemIDs) != 1 || lastDestinationEvent.SourceProblemIDs[0] != problems[0].ID {
		t.Fatalf("lost successive ancestry: %+v", lastDestinationEvent)
	}
}
