package patient

import (
	"context"
	"errors"
	"sonnda-api/internal/core/model"
	"time"
)

var (
	ErrCPFAlreadyExists = errors.New("CPF já cadastrado")
	ErrPatientNotFound  = errors.New("patient_not_found")
)

type Service interface {
	Create(ctx context.Context, input CreatePatientInput) (*model.Patient, error)
	GetByCPF(ctx context.Context, cpf string) (*model.Patient, error)
	UpdateByCPF(ctx context.Context, cpf string, input UpdatePatientInput) (*model.Patient, error)
	List(ctx context.Context, limit, offset int) ([]model.Patient, error)
}
type service struct {
	repo Repository
}

// CreatePatientAsDoctor implements Service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, input CreatePatientInput) (*model.Patient, error) {
	// 1. Evita CPF duplicado
	existing, err := s.repo.FindByCPF(ctx, input.CPF)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrCPFAlreadyExists
	}

	// 2. Parse de data
	birthDate, err := time.Parse("2006-01-02", input.BirthDate)
	if err != nil {
		return nil, errors.New("invalid_birth_date_format")
	}

	// 3. Monta o modelo
	p := &model.Patient{
		CPF:       input.CPF,
		CNS:       input.CNS,
		FullName:  input.FullName,
		BirthDate: birthDate,
		Gender:    input.Gender,
		Race:      input.Race,
		Phone:     input.Phone,
	}

	// 4. Persiste
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}

	return p, nil
}

func (s *service) GetByCPF(ctx context.Context, cpf string) (*model.Patient, error) {
	p, err := s.repo.FindByCPF(ctx, cpf)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPatientNotFound
	}
	return p, nil
}

func (s *service) UpdateByCPF(ctx context.Context, cpf string, input UpdatePatientInput) (*model.Patient, error) {
	p, err := s.repo.FindByCPF(ctx, cpf)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPatientNotFound
	}

	p.FullName = input.FullName
	p.Phone = input.Phone
	p.AvatarURL = input.AvatarURL

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}

	return p, nil
}

func (s *service) List(ctx context.Context, limit, offset int) ([]model.Patient, error) {
	return s.repo.List(ctx, limit, offset)
}
