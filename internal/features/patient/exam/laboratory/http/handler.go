// internal/features/patient/exam/laboratory/http/handler.go
package laboratoryhttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/google/uuid"

	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	laboratory "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory"
)

type labService interface {
	List(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]laboratory.LabReportSummaryOutput, error)
	ListFull(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]*laboratory.LabReportOutput, error)
	FindByID(ctx context.Context, reportID uuid.UUID) (*laboratory.LabReportOutput, error)
}

type Handler struct {
	svc           labService
	accessChecker patientaccess.Checker
}

type listLabReportsInput struct {
	PatientID uuid.UUID `path:"patientId" format:"uuid"`
	Expand    string    `query:"expand" enum:"full"`
	Include   string    `query:"include"`
	Limit     int       `query:"limit" default:"100" minimum:"1" maximum:"100"`
	Offset    int       `query:"offset" default:"0" minimum:"0"`
}

type labReportInput struct {
	LabReportID uuid.UUID `path:"labReportId" format:"uuid"`
}

type listLabReportsOutput struct {
	Body any
}

type labReportOutput struct {
	Body laboratory.LabReportOutput
}

func NewHandler(svc labService, accessChecker patientaccess.Checker) *Handler {
	return &Handler{svc: svc, accessChecker: accessChecker}
}

// RegisterHumaRoutes registers the laboratory-report operations.
func (h *Handler) RegisterHumaRoutes(registered huma.API, security []map[string][]string) {
	huma.Register(registered, huma.Operation{
		OperationID: "listPatientLabReports",
		Method:      http.MethodGet,
		Path:        "/patients/{patientId}/lab-reports",
		Summary:     "Listar laudos laboratoriais",
		Tags:        []string{"Lab reports"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
		Security:    security,
	}, h.listLabReports)

	huma.Register(registered, huma.Operation{
		OperationID: "getLabReport",
		Method:      http.MethodGet,
		Path:        "/lab-reports/{labReportId}",
		Summary:     "Obter laudo laboratorial",
		Tags:        []string{"Lab reports"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound},
		Security:    security,
	}, h.getLabReport)
}

func (h *Handler) listLabReports(ctx context.Context, input *listLabReportsInput) (*listLabReportsOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necess?ria")
	}
	if err := h.accessChecker.RequireAccess(ctx, currentAccount.ID, input.PatientID); err != nil {
		return nil, humaerror.From(err)
	}
	if shouldReturnFullLabsFor(input.Expand, input.Include) {
		list, err := h.svc.ListFull(ctx, input.PatientID, input.Limit, input.Offset)
		if err != nil {
			return nil, humaerror.From(err)
		}
		return &listLabReportsOutput{Body: list}, nil
	}
	list, err := h.svc.List(ctx, input.PatientID, input.Limit, input.Offset)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &listLabReportsOutput{Body: list}, nil
}

func (h *Handler) getLabReport(ctx context.Context, input *labReportInput) (*labReportOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necess?ria")
	}
	report, err := h.svc.FindByID(ctx, input.LabReportID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	if report == nil {
		return nil, huma.Error404NotFound("laudo não encontrado")
	}
	if err := h.accessChecker.RequireAccess(ctx, currentAccount.ID, report.PatientID); err != nil {
		return nil, humaerror.From(err)
	}
	return &labReportOutput{Body: *report}, nil
}

func shouldReturnFullLabsFor(expand, include string) bool {
	if strings.EqualFold(strings.TrimSpace(expand), "full") {
		return true
	}
	for _, raw := range strings.Split(include, ",") {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "full", "results", "panels", "test_results":
			return true
		}
	}
	return false
}
