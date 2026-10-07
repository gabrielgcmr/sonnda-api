// internal/features/patient/access/http/handler.go
package accesshttp

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	helpers "github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	"github.com/google/uuid"
)

type Handler struct {
	service patientaccess.Service
}

func NewHandler(service patientaccess.Service) *Handler {
	return &Handler{service: service}
}

type listPatientsInput struct {
	Limit  int `query:"limit" doc:"Quantidade máxima de pacientes retornados" default:"20"`
	Offset int `query:"offset" doc:"Quantidade de pacientes a pular" default:"0"`
}

type accessiblePatientResponse struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	FullName  string    `json:"full_name"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
}

type listPatientsResponse struct {
	Patients []accessiblePatientResponse `json:"patients"`
	Total    int64                       `json:"total"`
	Limit    int                         `json:"limit"`
	Offset   int                         `json:"offset"`
}

type listPatientsOutput struct {
	Body listPatientsResponse
}

// RegisterHumaRoutes registers the authenticated current-account patient list.
func (h *Handler) RegisterHumaRoutes(registered huma.API, security []map[string][]string) {
	huma.Register(registered, huma.Operation{
		OperationID: "listAccessiblePatients",
		Method:      http.MethodGet,
		Path:        "/me/patients",
		Summary:     "Listar pacientes acessíveis pela conta atual",
		Tags:        []string{"Patient access"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
		Security:    security,
	}, h.listForCurrentAccount)
}

func (h *Handler) listForCurrentAccount(ctx context.Context, input *listPatientsInput) (*listPatientsOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}

	result, err := h.service.ListForAccount(ctx, currentAccount.ID, input.Limit, input.Offset)
	if err != nil {
		return nil, humaerror.From(err)
	}

	patients := make([]accessiblePatientResponse, len(result.Patients))
	for i, patient := range result.Patients {
		patients[i] = accessiblePatientResponse{
			ID:        patient.ID,
			FullName:  patient.FullName,
			AvatarURL: patient.AvatarURL,
		}
	}

	return &listPatientsOutput{Body: listPatientsResponse{
		Patients: patients,
		Total:    result.Total,
		Limit:    result.Limit,
		Offset:   result.Offset,
	}}, nil
}
