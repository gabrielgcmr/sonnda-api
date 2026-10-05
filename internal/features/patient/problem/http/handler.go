// internal/features/patient/problem/http/handler.go
package problemhttp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
)

type Handler struct{ svc *problem.Service }

func NewHandler(svc *problem.Service) *Handler { return &Handler{svc: svc} }

type PatientInput struct {
	PatientID uuid.UUID `path:"patientId" format:"uuid"`
}

type ProblemInput struct {
	PatientInput
	ProblemID uuid.UUID `path:"problemId" format:"uuid"`
}

type PaginationInput struct {
	Limit  int `query:"limit" default:"20" minimum:"1" maximum:"100"`
	Offset int `query:"offset" default:"0" minimum:"0" maximum:"2147483647"`
}

type createInput struct {
	PatientInput
	Body struct {
		Name           string        `json:"name" minLength:"1" doc:"Nome livre do problema"`
		CID11          *ProblemCID11 `json:"cid11,omitempty"`
		Classification string        `json:"classification" enum:"acute,chronic"`
	}
}

type listInput struct {
	PatientInput
	PaginationInput
	ClinicalStatus       string `query:"clinical_status" default:"all" enum:"all,active,resolved"`
	AdministrativeStatus string `query:"administrative_status" default:"valid" enum:"valid,merged,entered_in_error,all"`
}

type historyInput struct {
	ProblemInput
	PaginationInput
}

type problemOutput struct{ Body ProblemResponse }
type createOutput struct {
	Location string `header:"Location"`
	Body     ProblemResponse
}
type listOutput struct{ Body ProblemPage }
type historyOutput struct{ Body ProblemHistoryPage }

func (h *Handler) RegisterHumaRoutes(api huma.API, security []map[string][]string) {
	base := "/patients/{patientId}/problems"
	operation := func(id, method, path, summary string) huma.Operation {
		return huma.Operation{
			OperationID: id, Method: method, Path: path, Summary: summary,
			Tags: []string{"Patient problems"}, Security: security,
			Errors: []int{400, 401, 403, 404, 422, 500},
		}
	}
	create := operation("createPatientProblem", http.MethodPost, base, "Criar problema do paciente (profissional com acesso)")
	create.DefaultStatus = http.StatusCreated
	huma.Register(api, create, h.create)
	list := operation("listPatientProblems", http.MethodGet, base, "Listar problemas do paciente")
	list.Description = "Registros válidos por padrão. Ordenação: updated_at DESC, id DESC. Paginação por limit/offset com has_more."
	huma.Register(api, list, h.list)
	huma.Register(api, operation("getPatientProblem", http.MethodGet, base+"/{problemId}", "Obter problema, inclusive unificado ou retificado"), h.get)
	history := operation("listPatientProblemHistory", http.MethodGet, base+"/{problemId}/history", "Consultar histórico do problema")
	history.Description = "Eventos em ordem de versão decrescente, com snapshots anteriores e posteriores. Problema inexistente ou de outro paciente retorna 404."
	huma.Register(api, history, h.history)
}

func (h *Handler) create(ctx context.Context, input *createInput) (*createOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	var cid *problemdomain.CID11
	if input.Body.CID11 != nil {
		cid = &problemdomain.CID11{Code: input.Body.CID11.Code, System: input.Body.CID11.System, Version: input.Body.CID11.Version}
	}
	p, err := h.svc.Create(ctx, actorID, input.PatientID, problem.CreateInput{
		Name: input.Body.Name, CID11: cid, Classification: problemdomain.Classification(input.Body.Classification),
	})
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &createOutput{Location: fmt.Sprintf("/patients/%s/problems/%s", p.PatientID, p.ID), Body: problemResponse(p)}, nil
}

func (h *Handler) get(ctx context.Context, input *ProblemInput) (*problemOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.Get(ctx, actorID, input.PatientID, input.ProblemID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &problemOutput{Body: problemResponse(p)}, nil
}

func (h *Handler) list(ctx context.Context, input *listInput) (*listOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.svc.List(ctx, actorID, input.PatientID, problem.ListFilter{
		Pagination:     problem.Pagination{Limit: input.Limit, Offset: input.Offset},
		ClinicalStatus: input.ClinicalStatus, AdministrativeStatus: input.AdministrativeStatus,
	})
	if err != nil {
		return nil, humaerror.From(err)
	}
	items := make([]ProblemResponse, len(page.Items))
	for i, p := range page.Items {
		items[i] = problemResponse(p)
	}
	return &listOutput{Body: ProblemPage{Items: items, Limit: page.Limit, Offset: page.Offset, HasMore: page.HasMore}}, nil
}

func (h *Handler) history(ctx context.Context, input *historyInput) (*historyOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.svc.History(ctx, actorID, input.PatientID, input.ProblemID, problem.Pagination{Limit: input.Limit, Offset: input.Offset})
	if err != nil {
		return nil, humaerror.From(err)
	}
	items := make([]ProblemHistoryEvent, len(page.Items))
	for i, event := range page.Items {
		items[i] = historyResponse(event)
	}
	return &historyOutput{Body: ProblemHistoryPage{Items: items, Limit: page.Limit, Offset: page.Offset, HasMore: page.HasMore}}, nil
}

func (h *Handler) actor(ctx context.Context) (uuid.UUID, error) {
	user, ok := helpers.GetCurrentUserFromContext(ctx)
	if !ok || user == nil {
		return uuid.Nil, huma.Error403Forbidden("conta registrada necessária")
	}
	if h == nil || h.svc == nil {
		return uuid.Nil, huma.Error500InternalServerError("serviço indisponível")
	}
	return user.ID, nil
}
