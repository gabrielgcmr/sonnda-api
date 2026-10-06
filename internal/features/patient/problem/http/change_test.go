// internal/features/patient/problem/http/change_test.go
package problemhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
)

func (r *testStore) Update(_ context.Context, version int64, p problemdomain.Problem, event problemdomain.HistoryEvent) error {
	r.calls++
	if r.beforeUpdate != nil {
		r.beforeUpdate()
	}
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.p.Version != version {
		return problem.ErrVersionConflict
	}
	r.p = p
	r.events = append(r.events, event)
	return nil
}

func changeTestStore(t *testing.T, classification problemdomain.Classification, resolved bool) *testStore {
	t.Helper()
	p, event, err := problemdomain.NewProblem(problemdomain.NewProblemParams{
		PatientID: uuid.New(), ActorAccountID: uuid.New(), Name: "Original",
		CID11:          &problemdomain.CID11{Code: "CA23", System: "ICD-11", Version: "2026"},
		Classification: classification, OccurredAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &testStore{p: p, events: []problemdomain.HistoryEvent{event}}
	if resolved {
		after := p.State
		after.ClinicalStatus = problemdomain.ClinicalStatusResolved
		next, event, err := p.Change(problemdomain.ChangeParams{Action: problemdomain.ActionResolved, After: after, ActorAccountID: p.CreatedByAccountID, OccurredAt: p.UpdatedAt.Add(time.Second)})
		if err != nil {
			t.Fatal(err)
		}
		store.p = next
		store.events = append(store.events, event)
	}
	return store
}

func changePath(store *testStore) string {
	return fmt.Sprintf("/patients/%s/problems/%s", store.p.PatientID, store.p.ID)
}

func TestChangeRoutesEnforcePermissionsAndRecordAuthorship(t *testing.T) {
	for _, kind := range []accountdomain.AccountType{accountdomain.AccountTypeBasicCare, accountdomain.AccountTypeProfessional} {
		for _, access := range []bool{false, true} {
			for _, operation := range []struct{ method, suffix, action, fields string }{
				{"PUT", "", "edited", `,"name":" Novo nome ","cid11":{"code":" BA00 ","system":"ICD-11","version":"2026"}`},
				{"PUT", "/classification", "classified", `,"classification":"chronic"`},
				{"POST", "/resolve", "resolved", ""},
				{"POST", "/reopen", "reopened", ""},
			} {
				t.Run(fmt.Sprintf("%s/%s/access=%t", kind, operation.action, access), func(t *testing.T) {
					store := changeTestStore(t, problemdomain.ClassificationAcute, operation.action == "reopened")
					before := store.p
					count := len(store.events)
					router, actorID := testRouter(store, kind, access)
					response := request(router, operation.method, changePath(store)+operation.suffix, fmt.Sprintf(`{"version":%d%s}`, before.Version, operation.fields))
					denied := !access || (kind == accountdomain.AccountTypeBasicCare && operation.action != "resolved")
					if denied {
						if response.Code != 403 || store.calls != 0 {
							t.Fatalf("status=%d calls=%d body=%s", response.Code, store.calls, response.Body.String())
						}
						return
					}
					var result ProblemResponse
					if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if response.Code != 200 || result.ID != before.ID || result.PatientID != before.PatientID || result.CreatedByAccountID != before.CreatedByAccountID || result.Version != before.Version+1 || len(store.events) != count+1 {
						t.Fatalf("status=%d result=%+v events=%d body=%s", response.Code, result, len(store.events), response.Body.String())
					}
					event := store.events[count]
					if string(event.Action) != operation.action || event.ActorAccountID != actorID || event.PatientID != before.PatientID || event.Before.Name != before.State.Name || event.Version != result.Version {
						t.Fatalf("invalid audit: %+v", event)
					}
					switch operation.action {
					case "edited":
						if result.Name != "Novo nome" || result.CID11.Code != "BA00" || event.Before.CID11.Code != "CA23" || result.Classification != "acute" || result.ClinicalStatus != "active" {
							t.Fatalf("invalid edited state: %+v", result)
						}
					case "classified":
						if result.Classification != "chronic" || result.ClinicalStatus != "active" {
							t.Fatalf("invalid classification: %+v", result)
						}
					case "resolved":
						if result.ClinicalStatus != "resolved" || event.Before.ClinicalStatus != problemdomain.ClinicalStatusActive {
							t.Fatalf("invalid resolution: %+v", result)
						}
					case "reopened":
						if result.ClinicalStatus != "active" || event.Before.ClinicalStatus != problemdomain.ClinicalStatusResolved {
							t.Fatalf("invalid reopening: %+v", result)
						}
					}
				})
			}
		}
	}
}

func TestChangesRejectInvalidPayloadsAndStatesWithoutAudit(t *testing.T) {
	for _, tc := range []struct {
		name, method, suffix, body string
		classification             problemdomain.Classification
		resolved                   bool
		administrative             problemdomain.AdministrativeStatus
	}{
		{name: "missing version", method: "PUT", body: `{"name":"Novo"}`},
		{name: "zero version", method: "POST", suffix: "/resolve", body: `{"version":0}`},
		{name: "missing name", method: "PUT", body: `{"version":1}`},
		{name: "blank name", method: "PUT", body: `{"version":1,"name":" "}`},
		{name: "incomplete CID", method: "PUT", body: `{"version":1,"name":"Novo","cid11":{"code":"X"}}`},
		{name: "blank CID", method: "PUT", body: `{"version":1,"name":"Novo","cid11":{"code":" ","system":"ICD-11","version":"2026"}}`},
		{name: "unknown classification", method: "PUT", suffix: "/classification", body: `{"version":1,"classification":"latent"}`},
		{name: "missing classification", method: "PUT", suffix: "/classification", body: `{"version":1}`},
		{name: "edit lifecycle", method: "PUT", body: `{"version":1,"name":"Novo","clinical_status":"resolved"}`},
		{name: "edit classification together", method: "PUT", body: `{"version":1,"name":"Novo","classification":"chronic"}`},
		{name: "reopen while classifying", method: "PUT", suffix: "/classification", body: `{"version":2,"classification":"chronic","clinical_status":"active"}`, resolved: true},
		{name: "spoof author", method: "POST", suffix: "/resolve", body: `{"version":1,"actor_account_id":"` + uuid.NewString() + `"}`},
		{name: "resolve chronic", method: "POST", suffix: "/resolve", body: `{"version":1}`, classification: problemdomain.ClassificationChronic},
		{name: "reclassify resolved", method: "PUT", suffix: "/classification", body: `{"version":2,"classification":"chronic"}`, resolved: true},
		{name: "resolve resolved", method: "POST", suffix: "/resolve", body: `{"version":2}`, resolved: true},
		{name: "reopen active", method: "POST", suffix: "/reopen", body: `{"version":1}`},
		{name: "same classification", method: "PUT", suffix: "/classification", body: `{"version":1,"classification":"acute"}`},
		{name: "same details", method: "PUT", body: `{"version":1,"name":"Original","cid11":{"code":"CA23","system":"ICD-11","version":"2026"}}`},
		{name: "edit rectified", method: "PUT", body: `{"version":1,"name":"Novo"}`, administrative: problemdomain.AdministrativeStatusEnteredInError},
		{name: "classify rectified", method: "PUT", suffix: "/classification", body: `{"version":1,"classification":"chronic"}`, administrative: problemdomain.AdministrativeStatusEnteredInError},
		{name: "resolve merged", method: "POST", suffix: "/resolve", body: `{"version":1}`, administrative: problemdomain.AdministrativeStatusMerged},
		{name: "reopen rectified", method: "POST", suffix: "/reopen", body: `{"version":2}`, resolved: true, administrative: problemdomain.AdministrativeStatusEnteredInError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			classification := tc.classification
			if classification == "" {
				classification = problemdomain.ClassificationAcute
			}
			store := changeTestStore(t, classification, tc.resolved)
			if tc.administrative != "" {
				store.p.State.AdministrativeStatus = tc.administrative
				if tc.administrative == problemdomain.AdministrativeStatusMerged {
					id := uuid.New()
					store.p.State.MergedIntoID = &id
				}
			}
			version, count := store.p.Version, len(store.events)
			router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
			response := request(router, tc.method, changePath(store)+tc.suffix, tc.body)
			if response.Code != 422 || store.p.Version != version || len(store.events) != count {
				t.Fatalf("status=%d version=%d events=%d body=%s", response.Code, store.p.Version, len(store.events), response.Body.String())
			}
		})
	}
	for _, kind := range []accountdomain.AccountType{accountdomain.AccountTypeBasicCare, accountdomain.AccountTypeProfessional} {
		store := changeTestStore(t, problemdomain.ClassificationChronic, false)
		router, _ := testRouter(store, kind, true)
		if response := request(router, "POST", changePath(store)+"/resolve", `{"version":1}`); response.Code != 422 {
			t.Fatalf("%s resolved chronic: %d %s", kind, response.Code, response.Body.String())
		}
	}
}

func TestEditRemovesCIDAndReopenAllowsChronicClassification(t *testing.T) {
	for _, cid := range []string{"", `,"cid11":null`} {
		store := changeTestStore(t, problemdomain.ClassificationAcute, true)
		router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
		response := request(router, "PUT", changePath(store), `{"version":2,"name":"Original"`+cid+`}`)
		if response.Code != 200 || store.p.State.CID11 != nil || store.p.State.ClinicalStatus != problemdomain.ClinicalStatusResolved || store.events[2].Before.CID11 == nil || store.events[2].After.CID11 != nil {
			t.Fatalf("CID removal: %d %s", response.Code, response.Body.String())
		}
		response = request(router, "POST", changePath(store)+"/reopen", `{"version":3}`)
		if response.Code != 200 {
			t.Fatalf("reopen: %d %s", response.Code, response.Body.String())
		}
		response = request(router, "PUT", changePath(store)+"/classification", `{"version":4,"classification":"chronic"}`)
		if response.Code != 200 || store.p.Version != 5 || store.p.State.Classification != problemdomain.ClassificationChronic || store.p.State.ClinicalStatus != problemdomain.ClinicalStatusActive || len(store.events) != 5 {
			t.Fatalf("classify after reopen: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestChangeConflictsCrossPatientAndSafeErrors(t *testing.T) {
	for _, operation := range []struct{ method, suffix, fields string }{
		{"PUT", "", `,"name":"Novo"`}, {"PUT", "/classification", `,"classification":"chronic"`},
		{"POST", "/resolve", ""}, {"POST", "/reopen", ""},
	} {
		store := changeTestStore(t, problemdomain.ClassificationAcute, operation.suffix == "/reopen")
		router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
		count := len(store.events)
		version := store.p.Version
		body := fmt.Sprintf(`{"version":%d%s}`, version, operation.fields)
		store.p.Version++
		response := request(router, operation.method, changePath(store)+operation.suffix, body)
		if response.Code != 409 || len(store.events) != count || store.calls != 1 {
			t.Fatalf("stale version: %d %s", response.Code, response.Body.String())
		}
		store.p.Version = version
		path := fmt.Sprintf("/patients/%s/problems/%s%s", uuid.New(), store.p.ID, operation.suffix)
		response = request(router, operation.method, path, body)
		if response.Code != 404 || len(store.events) != count {
			t.Fatalf("cross patient: %d %s", response.Code, response.Body.String())
		}
		path = fmt.Sprintf("/patients/%s/problems/%s%s", store.p.PatientID, uuid.New(), operation.suffix)
		response = request(router, operation.method, path, body)
		if response.Code != 404 {
			t.Fatalf("not found: %d %s", response.Code, response.Body.String())
		}
		store.updateErr = errors.New("secret database details")
		response = request(router, operation.method, changePath(store)+operation.suffix, body)
		if response.Code != 500 || len(store.events) != count || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("unsafe update error: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestResolveRejectsClassificationChangeBetweenReadAndWrite(t *testing.T) {
	store := changeTestStore(t, problemdomain.ClassificationAcute, false)
	router, professionalID := testRouter(store, accountdomain.AccountTypeProfessional, true)
	store.beforeUpdate = func() {
		store.beforeUpdate = nil
		response := request(router, "PUT", changePath(store)+"/classification", `{"version":1,"classification":"chronic"}`)
		if response.Code != 200 {
			t.Fatalf("concurrent classification: %d %s", response.Code, response.Body.String())
		}
	}
	careRouter, _ := testRouter(store, accountdomain.AccountTypeBasicCare, true)
	response := request(careRouter, "POST", changePath(store)+"/resolve", `{"version":1}`)
	if response.Code != 409 || store.p.Version != 2 || store.p.State.Classification != problemdomain.ClassificationChronic || store.p.State.ClinicalStatus != problemdomain.ClinicalStatusActive || len(store.events) != 2 || store.events[1].ActorAccountID != professionalID {
		t.Fatalf("resolution overwrote classification: %d %s %+v", response.Code, response.Body.String(), store.p)
	}
	response = request(careRouter, "POST", changePath(store)+"/resolve", `{"version":2}`)
	if response.Code != 422 || len(store.events) != 2 {
		t.Fatalf("fresh chronic resolution: %d %s", response.Code, response.Body.String())
	}
}

func TestRepeatedResolutionDoesNotAppendAudit(t *testing.T) {
	store := changeTestStore(t, problemdomain.ClassificationAcute, false)
	router, actorID := testRouter(store, accountdomain.AccountTypeBasicCare, true)
	path := changePath(store) + "/resolve"
	response := request(router, "POST", path, `{"version":1}`)
	if response.Code != 200 {
		t.Fatalf("initial resolution: %d %s", response.Code, response.Body.String())
	}
	for _, tc := range []struct {
		body   string
		status int
	}{{`{"version":1}`, 409}, {`{"version":2}`, 422}} {
		response = request(router, "POST", path, tc.body)
		if response.Code != tc.status || store.p.Version != 2 || len(store.events) != 2 || store.events[1].ActorAccountID != actorID {
			t.Fatalf("repeated resolution: %d %s events=%+v", response.Code, response.Body.String(), store.events)
		}
	}
}
