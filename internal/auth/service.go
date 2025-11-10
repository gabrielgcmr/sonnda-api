package auth

import (
	"context"
	"errors"

	"sonnda-api/internal/middleware"
	"sonnda-api/internal/user"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
)

type Service interface {
	Register(ctx context.Context, name, email, password string, role user.Role) (*user.User, error)
	Login(ctx context.Context, email, password string) (*user.User, string, error)
	Me(ctx context.Context, id uint) (*user.User, error)
}

type service struct {
	repo Repository
	jwt  *middleware.JWTManager
}

func NewService(repo Repository, jwt *middleware.JWTManager) Service {
	return &service{repo: repo, jwt: jwt}
}

func (s *service) Register(ctx context.Context, name, email, password string, role user.Role) (*user.User, error) {
	if existing, _ := s.repo.FindByEmail(ctx, email); existing != nil {
		return nil, ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &user.User{
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *service) Login(ctx context.Context, email, password string) (*user.User, string, error) {
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

func (s *service) Me(ctx context.Context, id uint) (*user.User, error) {
	return s.repo.FindByID(ctx, id)
}
