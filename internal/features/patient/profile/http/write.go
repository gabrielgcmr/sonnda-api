// internal/features/patient/profile/http/write.go
package profilehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	"github.com/google/uuid"
)

type nullableField[T any] struct {
	Set   bool
	Value *T
}

func (f *nullableField[T]) UnmarshalJSON(data []byte) error {
	f.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		f.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	f.Value = &value
	return nil
}

func (f nullableField[T]) Schema(registry huma.Registry) *huma.Schema {
	schema := registry.Schema(reflect.TypeFor[T](), true, "")
	schema.Nullable = true
	return schema
}

type updatePatientInput struct {
	PatientID uuid.UUID `path:"patientId" doc:"Identificador do paciente" format:"uuid"`
	Body      updatePatientRequest
}

type updatePatientRequest struct {
	FullName  nullableField[string] `json:"full_name,omitempty" doc:"Nome completo" minLength:"1"`
	Phone     nullableField[string] `json:"phone,omitempty" doc:"Telefone; vazio ou null remove o valor"`
	AvatarURL nullableField[string] `json:"avatar_url,omitempty" doc:"URL do avatar; vazio ou null remove o valor"`
	Gender    nullableField[string] `json:"gender,omitempty" doc:"Gênero"`
	Race      nullableField[string] `json:"race,omitempty" doc:"Raça/cor"`
	CNS       nullableField[string] `json:"cns,omitempty" doc:"CNS; vazio ou null remove o valor"`
}

func (h *Handler) updatePatient(ctx context.Context, input *updatePatientInput) (*patientOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok || currentAccount == nil {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	if !input.Body.hasUpdates() {
		return nil, huma.Error422UnprocessableEntity("informe ao menos um campo para atualização")
	}

	update, err := input.Body.toServiceInput()
	if err != nil {
		return nil, err
	}
	patient, err := h.svc.Update(ctx, currentAccount, input.PatientID, update)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &patientOutput{Body: patientResponseFromDomain(patient)}, nil
}

func (h *Handler) deletePatient(ctx context.Context, input *patientIDInput) (*struct{}, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok || currentAccount == nil {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	if err := h.svc.SoftDelete(ctx, currentAccount, input.PatientID); err != nil {
		return nil, humaerror.From(err)
	}
	return &struct{}{}, nil
}

func (r updatePatientRequest) hasUpdates() bool {
	return r.FullName.Set || r.Phone.Set || r.AvatarURL.Set || r.Gender.Set || r.Race.Set || r.CNS.Set
}

func (r updatePatientRequest) toServiceInput() (patientprofile.UpdateInput, error) {
	if r.FullName.Set && r.FullName.Value == nil {
		return patientprofile.UpdateInput{}, huma.Error422UnprocessableEntity("nome completo não pode ser nulo")
	}
	if r.Gender.Set && r.Gender.Value == nil {
		return patientprofile.UpdateInput{}, huma.Error422UnprocessableEntity("gênero não pode ser nulo")
	}
	if r.Race.Set && r.Race.Value == nil {
		return patientprofile.UpdateInput{}, huma.Error422UnprocessableEntity("raça não pode ser nula")
	}

	update := patientprofile.UpdateInput{
		FullName:  r.FullName.Value,
		Phone:     nullableStringUpdate(r.Phone),
		AvatarURL: nullableStringUpdate(r.AvatarURL),
		CNS:       nullableStringUpdate(r.CNS),
	}
	if r.Gender.Set {
		gender, err := parseGender(*r.Gender.Value)
		if err != nil {
			return patientprofile.UpdateInput{}, huma.Error422UnprocessableEntity("gênero inválido")
		}
		update.Gender = &gender
	}
	if r.Race.Set {
		race, err := parseRace(*r.Race.Value)
		if err != nil {
			return patientprofile.UpdateInput{}, huma.Error422UnprocessableEntity("raça inválida")
		}
		update.Race = &race
	}
	return update, nil
}

func nullableStringUpdate(field nullableField[string]) *string {
	if !field.Set {
		return nil
	}
	if field.Value != nil {
		return field.Value
	}
	empty := ""
	return &empty
}
