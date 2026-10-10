// internal/features/account/activation.go
package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type ActivationLimiter interface {
	CanAttempt(ctx context.Context, accountID uuid.UUID, origin string, limit int, window time.Duration) (bool, error)
	RecordFailure(ctx context.Context, accountID uuid.UUID, origin string, window time.Duration) error
}

type ProfessionalActivationOptions struct {
	PasswordHash string
	MaxAttempts  int
	Window       time.Duration
}

type ProfessionalActivationService struct {
	repository Repository
	limiter    ActivationLimiter
	options    ProfessionalActivationOptions
	configErr  error
}

func NewProfessionalActivationService(repository Repository, limiter ActivationLimiter, options ProfessionalActivationOptions) *ProfessionalActivationService {
	service := &ProfessionalActivationService{repository: repository, limiter: limiter, options: options}
	switch {
	case strings.TrimSpace(options.PasswordHash) == "":
		service.configErr = errors.New("professional activation password hash is missing")
	case options.MaxAttempts < 1:
		service.configErr = errors.New("professional activation max attempts must be positive")
	case options.Window <= 0:
		service.configErr = errors.New("professional activation window must be positive")
	default:
		_, service.configErr = bcrypt.Cost([]byte(options.PasswordHash))
		if service.configErr != nil {
			service.configErr = fmt.Errorf("invalid professional activation password hash: %w", service.configErr)
		}
	}
	return service
}

func (s *ProfessionalActivationService) Activate(ctx context.Context, accountID uuid.UUID, password, origin string) (*accountdomain.Account, error) {
	if accountID == uuid.Nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}
	if s == nil || s.repository == nil {
		return nil, apperr.Internal("habilitação profissional indisponível", errors.New("professional activation repository is not configured"))
	}

	user, err := s.repository.FindByID(ctx, accountID)
	if err != nil {
		return nil, mapRepoError("repository.FindByID", err)
	}
	if user == nil {
		return nil, accountNotFound()
	}
	if user.DeletedAt != nil {
		return nil, apperr.AccountDeactivated()
	}
	if user.AccountType == accountdomain.AccountTypeProfessional {
		return user, nil
	}
	if s.configErr != nil {
		return nil, apperr.Internal("habilitação profissional indisponível", s.configErr)
	}
	if s.limiter == nil {
		return nil, apperr.Internal("habilitação profissional indisponível", errors.New("professional activation limiter is not configured"))
	}

	origin = strings.TrimSpace(origin)
	if origin == "" {
		origin = "unknown"
	}
	allowed, err := s.limiter.CanAttempt(ctx, accountID, origin, s.options.MaxAttempts, s.options.Window)
	if err != nil {
		return nil, apperr.Internal("habilitação profissional indisponível", fmt.Errorf("activation limiter check: %w", err))
	}
	if !allowed {
		return nil, apperr.RateLimited("limite de tentativas excedido")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(s.options.PasswordHash), []byte(password)); err != nil {
		if recordErr := s.limiter.RecordFailure(ctx, accountID, origin, s.options.Window); recordErr != nil {
			return nil, apperr.Internal("habilitação profissional indisponível", fmt.Errorf("record activation failure: %w", recordErr))
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, apperr.Forbidden("senha de habilitação inválida")
		}
		return nil, apperr.Internal("habilitação profissional indisponível", fmt.Errorf("verify activation password: %w", err))
	}

	activated, err := s.repository.ActivateProfessional(ctx, accountID)
	if err != nil {
		return nil, mapRepoError("repository.ActivateProfessional", err)
	}
	return activated, nil
}
