package patient

import (
	"context"
	"errors"
	"sonnda-api/internal/core/model"
)

var (
	ErrCPFAlreadyExists      = errors.New("CPF já cadastrado")
	ErrUnauthorizedAccess    = errors.New("Não autorizado a acessar este recurso")
	ErrPatientEditRestricted = errors.New("Você não tem permissão para editar este perfil de paciente")
)

type Service interface {
	CreatePatientAsPatient(ctx context.Context, actorID uint, input CreatePatientInput) (*model.Patient, error)
	CreatePatient(ctx context.Context, actorID uint, input CreatePatientInput) (*model.Patient, error)
	Update(ctx context.Context, actorID uint, actorRole string, userID uint, input UpdatePatientInput) (*model.Patient, error)
	SelfUpdate(ctx context.Context, userID uint, input SelfUpdateInput) (*model.Patient, error)
	GetByUserID(ctx context.Context, actorID uint, actorRole string, userID uint) (*model.Patient, error)
	List(ctx context.Context, limit, offset int) ([]model.Patient, error)
}
type service struct {
	repo Repository
}

// CreatePatientAsDoctor implements Service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) CreatePatient(ctx context.Context, actorID uint, input CreatePatientInput) (*model.Patient, error) {
	// 1. Verifica se o CPF já existe
	existing, err := s.repo.FindByCPF(ctx, input.CPF)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrCPFAlreadyExists
	}

	// 2. Criar o PatientProfile
	patient := &model.Patient{
		UserID:    actorID,
		FullName:  input.FullName,
		BirthDate: input.BirthDate,
		Gender:    input.Gender,
		CPF:       input.CPF,
		Phone:     input.Phone,
	}
	if err := s.repo.Create(ctx, patient); err != nil {
		return nil, err
	}

	return patient, nil
}

func (s *service) CreatePatientAsPatient(ctx context.Context, actorID uint, input CreatePatientInput) (*model.Patient, error) {
	existing, err := s.repo.FindByCPF(ctx, input.CPF)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrCPFAlreadyExists
	}

	patient := &model.Patient{
		UserID:    input.UserID,
		FullName:  input.FullName,
		BirthDate: input.BirthDate,
		Gender:    input.Gender,
		CPF:       input.CPF,
		Phone:     input.Phone,
	}

	if err := s.repo.Create(ctx, patient); err != nil {
		return nil, err
	}
	return patient, nil
}

func (s *service) Update(ctx context.Context, actorID uint, actorRole string, userID uint, input UpdatePatientInput) (*model.Patient, error) {
	patient, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if actorRole == "PATIENT" && actorID != userID {
		return nil, ErrPatientEditRestricted
	}

	if actorRole == "DOCTOR" && actorID != userID {
		auths, err := s.repo.FindAuthorizations(ctx, userID)
		if err != nil || len(auths) == 0 || auths[0].Status != model.AuthApproved {
			return nil, ErrUnauthorizedAccess
		}
	}

	patient.FullName = input.FullName
	patient.Phone = input.Phone
	patient.AvatarURL = input.AvatarURL

	if err := s.repo.Update(ctx, patient); err != nil {
		return nil, err
	}
	return patient, nil
}

func (s *service) SelfUpdate(ctx context.Context, userID uint, input SelfUpdateInput) (*model.Patient, error) {
	patient, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	patient.Phone = input.Phone
	patient.AvatarURL = input.AvatarURL

	if err := s.repo.Update(ctx, patient); err != nil {
		return nil, err
	}
	return patient, nil
}

func (s *service) GetByUserID(ctx context.Context, actorID uint, actorRole string, userID uint) (*model.Patient, error) {
	patient, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if actorRole == "PATIENT" && actorID != userID {
		return nil, ErrUnauthorizedAccess
	}

	if actorRole == "DOCTOR" && actorID != userID {
		auths, err := s.repo.FindAuthorizations(ctx, userID)
		if err != nil || len(auths) == 0 || auths[0].Status != model.AuthApproved {
			return nil, ErrUnauthorizedAccess
		}
	}

	return patient, nil
}

func (s *service) List(ctx context.Context, limit, offset int) ([]model.Patient, error) {
	return s.repo.List(ctx, limit, offset)
}
