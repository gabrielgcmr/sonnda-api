package auth

import (
	"context"
	"errors"

	"sonnda-api/internal/core/jwt"
	"sonnda-api/internal/core/model"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
)

type Service interface {
	Register(ctx context.Context, name, email, password string, role model.Role) (*model.User, error)
	Login(ctx context.Context, email, password string) (*model.User, string, error)
	Me(ctx context.Context, id uint) (*model.User, error)
}

type service struct {
	repo Repository
	jwt  *jwt.JWTManager
}

func NewService(repo Repository, jwt *jwt.JWTManager) Service {
	return &service{repo: repo, jwt: jwt}
}

func (s *service) Register(ctx context.Context, name, email, password string, role model.Role) (*model.User, error) {
	if existing, _ := s.repo.FindByEmail(ctx, email); existing != nil {
		return nil, ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &model.User{
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *service) Login(ctx context.Context, email, password string) (*model.User, string, error) {
	u, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	token, err := s.jwt.Generate(u)
	if err != nil {
		return nil, "", err
	}

	return u, token, nil
}

func (s *service) Me(ctx context.Context, id uint) (*model.User, error) {
	return s.repo.FindByID(ctx, id)
}
