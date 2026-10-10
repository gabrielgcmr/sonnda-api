// internal/features/patient/profile/http/create.go
package profilehttp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	patientcreation "github.com/gabrielgcmr/sonnda/internal/application/usecase/patientcreation"
	"github.com/gabrielgcmr/sonnda/internal/domain/demographics"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	applog "github.com/gabrielgcmr/sonnda/internal/kernel/observability"
	"github.com/google/uuid"
)

type createPatientInput struct {
	Body createPatientRequest
}

type createPatientRequest struct {
	CPF          string  `json:"cpf" minLength:"1"`
	CNS          *string `json:"cns,omitempty"`
	FullName     string  `json:"full_name" minLength:"1"`
	BirthDate    string  `json:"birth_date" format:"date"`
	Gender       string  `json:"gender" minLength:"1"`
	Race         string  `json:"race" minLength:"1"`
	Phone        *string `json:"phone,omitempty"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
	RelationType string  `json:"relation_type" minLength:"1" enum:"self,family,caregiver,professional" doc:"Relação entre a conta atual e o paciente; professional exige uma conta profissional"`
}

type createPatientResponse struct {
	ID uuid.UUID `json:"id" format:"uuid"`
}

type createPatientOutput struct {
	Location string `header:"Location"`
	Body     createPatientResponse
}

func (h *Handler) createPatient(ctx context.Context, input *createPatientInput) (*createPatientOutput, error) {
	applog.FromContext(ctx).Info("patient_create")

	creatorAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok || creatorAccount == nil {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	if h == nil || h.creator == nil {
		return nil, humaerror.From(apperr.Internal("criação de paciente indisponível", errors.New("patient creation use case is not configured")))
	}

	birthDate, err := time.Parse(time.DateOnly, input.Body.BirthDate)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("data de nascimento inválida")
	}

	gender, err := parseGender(input.Body.Gender)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("gênero inválido")
	}
	race, err := parseRace(input.Body.Race)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("raça inválida")
	}

	avatarURL := ""
	if input.Body.AvatarURL != nil {
		avatarURL = *input.Body.AvatarURL
	}

	patient, err := h.creator.Execute(ctx, creatorAccount, patientcreation.Input{
		Profile: patientprofile.CreateInput{
			CPF:       input.Body.CPF,
			CNS:       input.Body.CNS,
			FullName:  input.Body.FullName,
			BirthDate: birthDate,
			Gender:    gender,
			Race:      race,
			Phone:     input.Body.Phone,
			AvatarURL: avatarURL,
		},
		Access: patientcreation.AccessInput{RelationType: input.Body.RelationType},
	})
	if err != nil {
		return nil, humaerror.From(err)
	}

	return &createPatientOutput{
		Location: "/patients/" + patient.ID.String(),
		Body:     createPatientResponse{ID: patient.ID},
	}, nil
}

func parseGender(value string) (demographics.Gender, error) {
	gender, err := demographics.ParseGender(value)
	if err != nil {
		return "", fmt.Errorf("invalid gender value %q: %w", value, err)
	}
	return gender, nil
}

func parseRace(value string) (demographics.Race, error) {
	race, err := demographics.ParseRace(value)
	if err != nil {
		return "", fmt.Errorf("invalid race value %q: %w", value, err)
	}
	return race, nil
}
