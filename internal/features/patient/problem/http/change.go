// internal/features/patient/problem/http/change.go
package problemhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
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

type rectifyInput struct {
	ProblemInput
	Body struct {
		Version int64  `json:"version" minimum:"1" doc:"Versão atual esperada do problema"`
		Reason  string `json:"reason" minLength:"1" doc:"Motivo da retificação"`
	}
}

type mergeCID11 struct {
	Set   bool
	Value *ProblemCID11
}

func (f *mergeCID11) UnmarshalJSON(data []byte) error {
	f.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		f.Value = nil
		return nil
	}
	var value ProblemCID11
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	f.Value = &value
	return nil
}

func (f mergeCID11) Schema(registry huma.Registry) *huma.Schema {
	schema := registry.Schema(reflect.TypeFor[ProblemCID11](), false, "")
	schema.Nullable = true
	return schema
}

type mergeSource struct {
	ID      uuid.UUID `json:"id" format:"uuid"`
	Version int64     `json:"version" minimum:"1"`
}

type mergeInput struct {
	ProblemInput
	Body struct {
		Version        int64         `json:"version" minimum:"1" doc:"Versão atual esperada do problema de destino"`
		Sources        []mergeSource `json:"sources" minItems:"1" doc:"Origens e suas versões atuais esperadas"`
		Name           string        `json:"name" minLength:"1" doc:"Nome final escolhido explicitamente"`
		CID11          mergeCID11    `json:"cid11" doc:"CID-11 final; null representa escolha explícita sem código"`
		Classification string        `json:"classification" enum:"acute,chronic"`
		ClinicalStatus string        `json:"clinical_status" enum:"active,resolved"`
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
	rectify := operation("rectifyPatientProblem", http.MethodPost, "/rectify", "Retificar problema registrado por engano (profissional com acesso)")
	rectify.Description += " reason é obrigatório. A retificação preserva conteúdo e histórico e remove o registro da listagem padrão."
	huma.Register(api, rectify, h.rectify)
	merge := operation("mergePatientProblems", http.MethodPost, "/merge", "Unificar problemas no destino (profissional com acesso)")
	merge.Description += " O problema da rota é o destino. sources, name, cid11, classification e clinical_status são escolhas explícitas. cid11 deve ser objeto ou null. Destino, origens e eventos são atualizados na mesma transação."
	huma.Register(api, merge, h.merge)
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

func (h *Handler) rectify(ctx context.Context, input *rectifyInput) (*problemOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.Rectify(ctx, actorID, input.PatientID, input.ProblemID, problem.RectifyInput{
		Version: input.Body.Version,
		Reason:  input.Body.Reason,
	})
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &problemOutput{Body: problemResponse(p)}, nil
}

func (h *Handler) merge(ctx context.Context, input *mergeInput) (*problemOutput, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	sources := make([]problem.MergeSourceInput, len(input.Body.Sources))
	for i, source := range input.Body.Sources {
		sources[i] = problem.MergeSourceInput{ID: source.ID, Version: source.Version}
	}
	var cid *problemdomain.CID11
	if input.Body.CID11.Value != nil {
		cid = &problemdomain.CID11{
			Code: input.Body.CID11.Value.Code, System: input.Body.CID11.Value.System,
			Version: input.Body.CID11.Value.Version,
		}
	}
	p, err := h.svc.Merge(ctx, actorID, input.PatientID, input.ProblemID, problem.MergeInput{
		Version: input.Body.Version, Sources: sources, Name: input.Body.Name,
		CID11: cid, CID11Set: input.Body.CID11.Set,
		Classification: problemdomain.Classification(input.Body.Classification),
		ClinicalStatus: problemdomain.ClinicalStatus(input.Body.ClinicalStatus),
	})
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &problemOutput{Body: problemResponse(p)}, nil
}
