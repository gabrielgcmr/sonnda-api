package patient

import (
	"errors"
	"fmt"

	"github.com/gabrielgcmr/sonnda-api/internal/patient/dto"
	"github.com/gabrielgcmr/sonnda-api/internal/patient/utils"

	"gorm.io/gorm"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Register cria um novo usuário a partir do DTO de registro
func (s *Service) Register(input dto.RegisterInput) (*Patient, error) {
	// 1) Checa se já existe por email
	if _, err := s.repo.FindByEmail(input.Email); err == nil {
		return nil, errors.New("este e-mail já está em uso")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("checando email existente: %w", err)
	}

	// 2) Gera hash da senha
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	// 3) Monta o User para persistir
	u := &Patient{
		CPF:          input.CPF,
		CNS:          ptrString(input.CNS),
		FullName:     input.FullName,
		Email:        ptrString(input.Email),
		PasswordHash: hash,
		Phone:        ptrString(input.Phone),
	}

	// 4) Persiste
	if err := s.repo.Create(u); err != nil {
		return nil, fmt.Errorf("salvando usuário: %w", err)
	}

	return u, nil
}

// Login autentica o usuário e retorna o modelo (você pode também retornar um token)
func (s *Service) Login(email, password string) (*Patient, error) {
	u, err := s.repo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("e-mail ou senha inválidos")
		}
		return nil, fmt.Errorf("buscando usuário: %w", err)
	}

	if !utils.CheckPasswordHash(password, u.PasswordHash) {
		return nil, errors.New("e-mail ou senha inválidos")
	}

	return u, nil
}

func (s *Service) GetByID(id int) (*Patient, error) {
	return s.repo.FindByID(id)
}

// helper para ponteiros
func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
