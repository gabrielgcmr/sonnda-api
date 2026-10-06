// internal/features/patient/problem/http/handler_test.go
package problemhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type testStore struct {
	problem.Repository
	p            problemdomain.Problem
	events       []problemdomain.HistoryEvent
	calls        int
	filter       problem.ListFilter
	page         problem.Pagination
	err          error
	updateErr    error
	beforeUpdate func()
}

func (r *testStore) Create(_ context.Context, p problemdomain.Problem, event problemdomain.HistoryEvent) error {
	r.calls++
	if r.err != nil {
		return r.err
	}
	r.p, r.events = p, []problemdomain.HistoryEvent{event}
	return nil
}
func (r *testStore) Get(_ context.Context, patientID, id uuid.UUID) (problemdomain.Problem, error) {
	r.calls++
	if r.err != nil {
		return problemdomain.Problem{}, r.err
	}
	if r.p.ID != id || r.p.PatientID != patientID {
		return problemdomain.Problem{}, problem.ErrNotFound
	}
	return r.p, nil
}
func (r *testStore) List(_ context.Context, _ uuid.UUID, filter problem.ListFilter) ([]problemdomain.Problem, error) {
	r.calls++
	r.filter = filter
	if r.err != nil {
		return nil, r.err
	}
	if r.p.ID == uuid.Nil {
		return nil, nil
	}
	return []problemdomain.Problem{r.p, r.p}, nil
}
func (r *testStore) ListHistory(_ context.Context, _, _ uuid.UUID, page problem.Pagination) ([]problemdomain.HistoryEvent, error) {
	r.calls++
	r.page = page
	return r.events, r.err
}

type testAccountLookup struct{ account *accountdomain.User }

func (a testAccountLookup) FindByID(context.Context, uuid.UUID) (*accountdomain.User, error) {
	return a.account, nil
}

type testAccess struct{ allowed bool }

func (a testAccess) RequireAccess(context.Context, uuid.UUID, uuid.UUID) error {
	if !a.allowed {
		return apperr.Forbidden("acesso negado")
	}
	return nil
}

func testRouter(store *testStore, kind accountdomain.AccountType, hasAccess bool) (http.Handler, uuid.UUID) {
	gin.SetMode(gin.TestMode)
	actorID := uuid.New()
	// The middleware's account type must not override the persisted account type.
	account := &accountdomain.User{ID: actorID, AccountType: kind}
	authorizer := authz.NewProblemAuthorizer(authz.NewPatientContextResolver(testAccountLookup{account}, testAccess{hasAccess}))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentUser(c.Request.Context(), &accountdomain.User{ID: actorID, AccountType: accountdomain.AccountTypeProfessional}))
		c.Next()
	})
	api := humagin.New(router, huma.DefaultConfig("test", "test"))
	NewHandler(problem.New(store, authorizer)).RegisterHumaRoutes(api, nil)
	return router, actorID
}

func request(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestProblemRoutesEnforcePersistedPermissionsBeforeStorage(t *testing.T) {
	for _, kind := range []accountdomain.AccountType{accountdomain.AccountTypeBasicCare, accountdomain.AccountTypeProfessional} {
		for _, access := range []bool{false, true} {
			for _, operation := range []string{"create", "list", "get", "history"} {
				t.Run(string(kind)+"/"+operation+"/access="+map[bool]string{true: "yes", false: "no"}[access], func(t *testing.T) {
					patientID := uuid.New()
					p, event, err := problemdomain.NewProblem(problemdomain.NewProblemParams{PatientID: patientID, ActorAccountID: uuid.New(), Name: "Teste", Classification: problemdomain.ClassificationAcute, OccurredAt: time.Now()})
					if err != nil {
						t.Fatal(err)
					}
					store := &testStore{p: p, events: []problemdomain.HistoryEvent{event}}
					router, _ := testRouter(store, kind, access)
					path := "/patients/" + patientID.String() + "/problems"
					method, body := "GET", ""
					expected := 200
					switch operation {
					case "create":
						method, body, expected = "POST", `{"name":"Teste","classification":"acute"}`, 201
					case "get":
						path += "/" + p.ID.String()
					case "history":
						path += "/" + p.ID.String() + "/history"
					}
					denied := !access || (operation == "create" && kind == accountdomain.AccountTypeBasicCare)
					if denied {
						expected = 403
					}
					response := request(router, method, path, body)
					if response.Code != expected {
						t.Fatalf("status=%d want=%d body=%s", response.Code, expected, response.Body.String())
					}
					if denied && store.calls != 0 {
						t.Fatalf("unauthorized storage calls: %d", store.calls)
					}
				})
			}
		}
	}
}

func TestCreateAndHistoryPreserveAuthorshipAndSnapshot(t *testing.T) {
	store := &testStore{}
	router, actorID := testRouter(store, accountdomain.AccountTypeProfessional, true)
	patientID := uuid.New()
	base := "/patients/" + patientID.String() + "/problems"
	response := request(router, "POST", base, `{"name":"  Hipertensão  ","classification":"chronic","cid11":{"code":"BA00","system":"ICD-11","version":"2026"}}`)
	if response.Code != 201 {
		t.Fatalf("%d: %s", response.Code, response.Body.String())
	}
	var created ProblemResponse
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == uuid.Nil || created.PatientID != patientID || created.CreatedByAccountID != actorID || created.Name != "Hipertensão" || created.Version != 1 || created.ClinicalStatus != "active" || created.AdministrativeStatus != "valid" || created.CID11.Code != "BA00" {
		t.Fatalf("unexpected creation: %+v", created)
	}
	if response.Header().Get("Location") != base+"/"+created.ID.String() {
		t.Fatalf("Location=%s", response.Header().Get("Location"))
	}
	response = request(router, "GET", response.Header().Get("Location")+"/history", "")
	if response.Code != 200 {
		t.Fatalf("%d: %s", response.Code, response.Body.String())
	}
	var history ProblemHistoryPage
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 1 || history.Items[0].BeforeSnapshot != nil || history.Items[0].AfterSnapshot.Name != created.Name || history.Items[0].ActorAccountID != actorID || history.Items[0].SourceProblemIDs == nil || history.Items[0].Action != "created" {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestProblemHTTPValidation(t *testing.T) {
	for _, body := range []string{
		`{"name":"Teste"}`, `{"name":"Teste","classification":"latent"}`,
		`{"name":"   ","classification":"acute"}`, `{"name":"Teste","classification":"acute","cid11":{"code":"X"}}`,
		`{"name":"Teste","classification":"acute","cid11":{"code":" ","system":"ICD-11","version":"2026"}}`,
		`{"name":"Teste","classification":"acute","clinical_status":"resolved"}`,
		`{"name":"Teste","classification":"acute","created_by_account_id":"` + uuid.NewString() + `"}`,
	} {
		t.Run(body, func(t *testing.T) {
			store := &testStore{}
			router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
			response := request(router, "POST", "/patients/"+uuid.NewString()+"/problems", body)
			if response.Code != 422 || store.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, store.calls, response.Body.String())
			}
		})
	}
	for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?offset=2147483648", "?clinical_status=unknown", "?administrative_status=unknown"} {
		store := &testStore{}
		router, _ := testRouter(store, accountdomain.AccountTypeProfessional, true)
		response := request(router, "GET", "/patients/"+uuid.NewString()+"/problems"+suffix, "")
		if response.Code != 422 || store.calls != 0 {
			t.Fatalf("%s: %d, calls=%d", suffix, response.Code, store.calls)
		}
	}
}

func TestProblemQueriesDefaultsPaginationAndErrors(t *testing.T) {
	patientID, problemID := uuid.New(), uuid.New()
	store := &testStore{p: problemdomain.Problem{ID: problemID, PatientID: patientID}}
	router, _ := testRouter(store, accountdomain.AccountTypeBasicCare, true)
	base := "/patients/" + patientID.String() + "/problems"
	response := request(router, "GET", base+"?limit=1&offset=2", "")
	var page ProblemPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || len(page.Items) != 1 || !page.HasMore || page.Limit != 1 || page.Offset != 2 || store.filter.Limit != 2 || store.filter.Offset != 2 || store.filter.AdministrativeStatus != "valid" || store.filter.ClinicalStatus != "all" {
		t.Fatalf("page=%+v filter=%+v status=%d", page, store.filter, response.Code)
	}
	response = request(router, "GET", base+"?administrative_status=all&clinical_status=resolved", "")
	if response.Code != 200 || store.filter.AdministrativeStatus != "all" || store.filter.ClinicalStatus != "resolved" {
		t.Fatal("filters not forwarded")
	}
	for _, suffix := range []string{"", "/history"} {
		response = request(router, "GET", "/patients/"+uuid.NewString()+"/problems/"+problemID.String()+suffix, "")
		if response.Code != 404 {
			t.Fatalf("cross-patient %s: %d %s", suffix, response.Code, response.Body.String())
		}
	}
	store.p = problemdomain.Problem{}
	response = request(router, "GET", base, "")
	if !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("empty page: %s", response.Body.String())
	}
	store.err = errors.New("database secret connection details")
	response = request(router, "GET", base, "")
	if response.Code != 500 || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unsafe database error: %d %s", response.Code, response.Body.String())
	}
}
