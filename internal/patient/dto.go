package patient

import (
	"sonnda-api/internal/core/model"
)

type CreatePatientInput struct {
	CPF       string       `json:"cpf" binding:"required,len=11"`
	CNS       *string      `json:"cns"`
	FullName  string       `json:"full_name" binding:"required,min=2"`
	BirthDate string       `json:"birth_date" binding:"required"`
	Gender    model.Gender `json:"gender" binding:"required"`
	Race      model.Race   `json:"race"`
	Phone     *string      `json:"phone,omitempty"`
}

type UpdatePatientInput struct {
	FullName  string  `json:"full_name" binding:"required"`
	Phone     *string `json:"phone,omitempty"`
	AvatarURL string  `json:"avatar_url"`
}
