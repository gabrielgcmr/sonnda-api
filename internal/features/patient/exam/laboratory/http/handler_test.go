// internal/features/patient/exam/laboratory/http/handler_test.go
package laboratoryhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	laboratory "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory"
)

type fakeLabService struct {
	listCalled     bool
	listFullCalled bool
	report         *laboratory.LabReportOutput
	account        *accountdomain.Account
	reportID       uuid.UUID
}

func (f *fakeLabService) List(context.Context, *accountdomain.Account, uuid.UUID, int, int) ([]laboratory.LabReportSummaryOutput, error) {
	f.listCalled = true
	return []laboratory.LabReportSummaryOutput{}, nil
}

func (f *fakeLabService) ListFull(context.Context, *accountdomain.Account, uuid.UUID, int, int) ([]*laboratory.LabReportOutput, error) {
	f.listFullCalled = true
	return []*laboratory.LabReportOutput{}, nil
}

func (f *fakeLabService) FindByID(_ context.Context, account *accountdomain.Account, reportID uuid.UUID) (*laboratory.LabReportOutput, error) {
	f.account, f.reportID = account, reportID
	return f.report, nil
}

func TestListLabsReturnsSummaryByDefault(t *testing.T) {
	assertListMode(t, "", false)
}

func TestListLabsCanReturnFullResults(t *testing.T) {
	assertListMode(t, "?include=results", true)
	assertListMode(t, "?expand=full", true)
}

func TestGetLabReportDelegatesAuthorizationContextToService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	patientID := uuid.New()
	reportID := uuid.New()
	svc := &fakeLabService{report: &laboratory.LabReportOutput{ID: reportID, PatientID: patientID}}
	handler := NewHandler(svc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}))
		c.Next()
	})
	handler.RegisterHumaRoutes(humagin.New(router, huma.DefaultConfig("test", "test")), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/lab-reports/"+reportID.String(), nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if svc.account == nil || svc.account.ID == uuid.Nil || svc.reportID != reportID {
		t.Fatalf("authorization context not delegated: account=%+v report=%s", svc.account, svc.reportID)
	}
}

func assertListMode(t *testing.T, query string, wantFull bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := &fakeLabService{}
	handler := NewHandler(svc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(helpers.ContextWithCurrentAccount(c.Request.Context(), &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}))
		c.Next()
	})
	handler.RegisterHumaRoutes(humagin.New(router, huma.DefaultConfig("test", "test")), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/patients/"+uuid.NewString()+"/lab-reports"+query, nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if svc.listFullCalled != wantFull || svc.listCalled == wantFull {
		t.Fatalf("summary called=%v, full called=%v; want full=%v", svc.listCalled, svc.listFullCalled, wantFull)
	}
}
