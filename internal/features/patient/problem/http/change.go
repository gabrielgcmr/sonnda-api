// internal/features/patient/problem/http/change.go
package problemhttp

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
)

type editInput struct {
	ProblemInput
	Body struct {
		Version int64         `json:"version" minimum:"1" doc:"Versão atual esperada do problema"`
		Name    string        `json:"name" minLength:"1"`
		CID11   *ProblemCID11 `json:"cid11,omitempty" doc:"CID-11 final; omitido ou null remove o código anterior"`
	}
}

type classifyInput struct {
	ProblemInput
	Body struct {
		Version        int64  `json:"version" minimum:"1" doc:"Versão atual esperada do problema"`
		Classification string `json:"classification" enum:"acute,chronic"`
	}
}

type transitionInput struct {
	ProblemInput
	Body struct {
		Version int64 `json:"version" minimum:"1" doc:"Versão atual esperada do problema"`
	}
}

func (h *Handler) registerChangeRoutes(api huma.API, security []map[string][]string) {
	base := "/patients/{patientId}/problems/{problemId}"
	operation := func(id, method, suffix, summary string) huma.Operation {
		return huma.Operation{
			OperationID: id, Method: method, Path: base + suffix, Summary: summary,
			Tags: []string{"Patient problems"}, Security: security,
			Errors:      []int{400, 401, 403, 404, 409, 422, 500},
			Description: "Exige version atual. Conflito de versão retorna 409; consulte novamente antes de decidir repetir. Grava alteração e auditoria atomicamente. Registros unificados ou retificados não podem ser alterados.",
		}
	}
	edit := operation("editPatientProblem", http.MethodPut, "", "Substituir nome e CID-11 (profissional com acesso)")
	edit.Description += " name é obrigatório; cid11 omitido ou null remove o código anterior. Não altera classificação ou situação clínica."
	huma.Register(api, edit, h.edit)
	huma.Register(api, operation("classifyPatientProblem", http.MethodPut, "/classification", "Classificar problema (profissional com acesso)"), h.classify)
	huma.Register(api, operation("resolvePatientProblem", http.MethodPost, "/resolve", "Resolver problema agudo ativo (qualquer conta com acesso)"), h.resolve)
	huma.Register(api, operation("reopenPatientProblem", http.MethodPost, "/reopen", "Reabrir problema resolvido (profissional com acesso)"), h.reopen)
}

func (h *Handler) edit(ctx context.Context, input *editInput) (*problemOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	var cid *problemdomain.CID11
	if input.Body.CID11 != nil {
		cid = &problemdomain.CID11{Code: input.Body.CID11.Code, System: input.Body.CID11.System, Version: input.Body.CID11.Version}
	}
	p, err := h.svc.Edit(ctx, actorID, input.PatientID, input.ProblemID, problem.EditInput{
		Version: input.Body.Version, Name: input.Body.Name, CID11: cid,
	})
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &problemOutput{Body: problemResponse(p)}, nil
}

func (h *Handler) classify(ctx context.Context, input *classifyInput) (*problemOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.Classify(ctx, actorID, input.PatientID, input.ProblemID, problem.ClassifyInput{
		Version: input.Body.Version, Classification: problemdomain.Classification(input.Body.Classification),
	})
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &problemOutput{Body: problemResponse(p)}, nil
}

func (h *Handler) resolve(ctx context.Context, input *transitionInput) (*problemOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.Resolve(ctx, actorID, input.PatientID, input.ProblemID, input.Body.Version)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &problemOutput{Body: problemResponse(p)}, nil
}

func (h *Handler) reopen(ctx context.Context, input *transitionInput) (*problemOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.Reopen(ctx, actorID, input.PatientID, input.ProblemID, input.Body.Version)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &problemOutput{Body: problemResponse(p)}, nil
}
