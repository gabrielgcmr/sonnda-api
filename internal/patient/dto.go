package patient

import "time"

type CreatePatientInput struct {
	UserID    uint      `json:"user_id" binding:"required"`
	FullName  string    `json:"full_name" binding:"required,min=2"`
	BirthDate time.Time `json:"birth_date" binding:"required"`
	Gender    Gender    `json:"gender" binding:"required"`
	CPF       string    `json:"cpf" binding:"required,len=11"`
	Phone     *string   `json:"phone,omitempty"`
}

type SelfUpdateInput struct {
	Phone     *string `json:"phone,omitempty"`
	AvatarURL string  `json:"avatar_url,omitempty"`
}

type UpdatePatientInput struct {
	FullName  string  `json:"full_name" binding:"required"`
	Phone     *string `json:"phone,omitempty"`
	AvatarURL string  `json:"avatar_url"`
}
