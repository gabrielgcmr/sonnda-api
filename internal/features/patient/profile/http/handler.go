// internal/features/patient/profile/http/handler.go
package profilehttp

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	helpers "github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	patientcreation "github.com/gabrielgcmr/sonnda/internal/application/usecase/patientcreation"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/google/uuid"
)

type patientService interface {
	Get(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*profiledomain.Patient, error)
	Update(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID, input patientprofile.UpdateInput) (*profiledomain.Patient, error)
	SoftDelete(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) error
	ListMyPatients(ctx context.Context, currentAccount *accountdomain.Account, limit, offset int) ([]*profiledomain.Patient, error)
}

type Handler struct {
	svc     patientService
	creator patientcreation.UseCase
}

type patientIDInput struct {
	PatientID uuid.UUID `path:"patientId" doc:"Identificador do paciente" format:"uuid"`
}

type patientResponse struct {
	ID          uuid.UUID  `json:"id" format:"uuid"`
	OwnerUserID *uuid.UUID `json:"owner_user_id,omitempty" format:"uuid"`
	CPF         string     `json:"cpf"`
	CNS         *string    `json:"cns,omitempty"`
	FullName    string     `json:"full_name"`
	BirthDate   time.Time  `json:"birth_date"`
	Gender      string     `json:"gender"`
	Race        string     `json:"race"`
	AvatarURL   string     `json:"avatar_url"`
	Phone       *string    `json:"phone,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type patientOutput struct {
	Body patientResponse
}

type patientListOutput struct {
	Body []patientResponse
}

func NewHandler(svc patientService, creator patientcreation.UseCase) *Handler {
	return &Handler{svc: svc, creator: creator}
}

// RegisterHumaRoutes registers the patient profile lifecycle in the onboarded-account group.
func (h *Handler) RegisterHumaRoutes(registered huma.API, security []map[string][]string) {
	huma.Register(registered, huma.Operation{
		OperationID:   "createPatient",
		Method:        http.MethodPost,
		Path:          "/patients",
		Summary:       "Criar paciente e conceder acesso inicial à conta atual",
		Tags:          []string{"Patients"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError},
		Security:      security,
	}, h.createPatient)

	huma.Register(registered, huma.Operation{
		OperationID: "listPatients",
		Method:      http.MethodGet,
		Path:        "/patients",
		Summary:     "Listar pacientes acessíveis pela conta atual",
		Tags:        []string{"Patients"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
		Security:    security,
	}, h.listPatients)

	huma.Register(registered, huma.Operation{
		OperationID: "getPatient",
		Method:      http.MethodGet,
		Path:        "/patients/{patientId}",
		Summary:     "Obter perfil de um paciente",
		Tags:        []string{"Patients"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity},
		Security:    security,
	}, h.getPatient)

	huma.Register(registered, huma.Operation{
		OperationID: "updatePatient",
		Method:      http.MethodPatch,
		Path:        "/patients/{patientId}",
		Summary:     "Atualizar o perfil de um paciente",
		Tags:        []string{"Patients"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError},
		Security:    security,
	}, h.updatePatient)

	huma.Register(registered, huma.Operation{
		OperationID:   "deletePatient",
		Method:        http.MethodDelete,
		Path:          "/patients/{patientId}",
		Summary:       "Desativar um paciente",
		Tags:          []string{"Patients"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError},
		Security:      security,
	}, h.deletePatient)
}

func (h *Handler) getPatient(ctx context.Context, input *patientIDInput) (*patientOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}

	patient, err := h.svc.Get(ctx, currentAccount, input.PatientID)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &patientOutput{Body: patientResponseFromDomain(patient)}, nil
}

func (h *Handler) listPatients(ctx context.Context, _ *struct{}) (*patientListOutput, error) {
	if h == nil || h.svc == nil {
		return nil, huma.Error500InternalServerError("serviço indisponível")
	}

	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}

	patients, err := h.svc.ListMyPatients(ctx, currentAccount, 100, 0)
	if err != nil {
		return nil, humaerror.From(err)
	}

	response := make([]patientResponse, len(patients))
	for i, patient := range patients {
		response[i] = patientResponseFromDomain(patient)
	}
	return &patientListOutput{Body: response}, nil
}

func patientResponseFromDomain(patient *profiledomain.Patient) patientResponse {
	return patientResponse{
		ID:          patient.ID,
		OwnerUserID: patient.OwnerUserID,
		CPF:         patient.CPF,
		CNS:         patient.CNS,
		FullName:    patient.FullName,
		BirthDate:   patient.BirthDate,
		Gender:      string(patient.Gender),
		Race:        string(patient.Race),
		AvatarURL:   patient.AvatarURL,
		Phone:       patient.Phone,
		CreatedAt:   patient.CreatedAt,
		UpdatedAt:   patient.UpdatedAt,
	}
}
