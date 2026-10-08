// internal/features/patient/profile/domain/patient.go
package profiledomain

import (
	"strings"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/domain/demographics"

	"github.com/google/uuid"
)

type Patient struct {
	ID          uuid.UUID           `json:"id"`
	OwnerUserID *uuid.UUID          `json:"owner_user_id,omitempty"`
	CPF         string              `json:"cpf"`
	CNS         *string             `json:"cns,omitempty"`
	FullName    string              `json:"full_name"`
	BirthDate   time.Time           `json:"birth_date"`
	Gender      demographics.Gender `json:"gender"`
	Race        demographics.Race   `json:"race"`

	AvatarURL string    `json:"avatar_url"`
	Phone     *string   `json:"phone,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type NewPatientParams struct {
	UserID    *uuid.UUID
	CPF       string
	CNS       *string
	FullName  string
	BirthDate time.Time
	Gender    demographics.Gender
	Race      demographics.Race
	Phone     *string
	AvatarURL string
}

func (p *NewPatientParams) Normalize() {
	p.CPF = demographics.CleanDigits(p.CPF)
	p.FullName = strings.TrimSpace(p.FullName)
	p.AvatarURL = strings.TrimSpace(p.AvatarURL)

	if p.CNS != nil {
		cns := strings.TrimSpace(*p.CNS)
		if cns == "" {
			p.CNS = nil
		} else {
			p.CNS = &cns
		}
	}

	if p.Phone != nil {
		phone := strings.TrimSpace(*p.Phone)
		if phone == "" {
			p.Phone = nil
		} else {
			p.Phone = &phone
		}
	}
}

func NewPatient(params NewPatientParams) (*Patient, error) {
	params.Normalize()

	now := time.Now().UTC()
	p := &Patient{
		ID:          uuid.Must(uuid.NewV7()),
		CPF:         params.CPF,
		CNS:         params.CNS,
		FullName:    params.FullName,
		BirthDate:   params.BirthDate.UTC(),
		OwnerUserID: params.UserID,
		Gender:      params.Gender,
		Race:        params.Race,
		AvatarURL:   params.AvatarURL,
		Phone:       params.Phone,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := p.Validate(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Patient) Validate() error {
	if p.FullName == "" {
		return ErrInvalidFullName
	}
	if p.BirthDate.IsZero() || p.BirthDate.After(time.Now().UTC()) {
		return demographics.ErrInvalidBirthDate
	}
	if p.CPF == "" || len(p.CPF) != 11 {
		return demographics.ErrInvalidCPF
	}
	return nil
}

func (p *Patient) ApplyUpdate(
	fullName *string,
	phone *string,
	avatarURL *string,
	gender *demographics.Gender,
	race *demographics.Race,
	cns *string,
) {
	if fullName != nil {
		name := strings.TrimSpace(*fullName)
		if name != "" {
			p.FullName = name
		}
	}

	if phone != nil {
		pval := strings.TrimSpace(*phone)
		if pval == "" {
			p.Phone = nil
		} else {
			p.Phone = &pval
		}
	}

	if avatarURL != nil {
		p.AvatarURL = strings.TrimSpace(*avatarURL)
	}

	if gender != nil {
		p.Gender = *gender
	}

	if race != nil {
		p.Race = *race
	}

	if cns != nil {
		value := strings.TrimSpace(*cns)
		if value == "" {
			p.CNS = nil
		} else {
			p.CNS = &value
		}
	}

	p.UpdatedAt = time.Now().UTC()
}
