// internal/features/account/error_map.go
package account

import (
	"errors"
	"fmt"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
)

func mapDomainError(err error) error {
	switch {
	case errors.Is(err, accountdomain.ErrInvalidAuthIssuer),
		errors.Is(err, accountdomain.ErrInvalidAuthSubject),
		errors.Is(err, accountdomain.ErrInvalidAccountID),
		errors.Is(err, accountdomain.ErrInvalidFullName),
		errors.Is(err, accountdomain.ErrInvalidAccountType),
		errors.Is(err, accountdomain.ErrInvalidBirthDate),
		errors.Is(err, accountdomain.ErrInvalidCPF),
		errors.Is(err, accountdomain.ErrInvalidPhone):
		return &apperr.AppError{
			Kind:    apperr.VALIDATION_FAILED,
			Message: "dados do usuário inválidos",
			Cause:   err,
		}

	default:
		return &apperr.AppError{
			Kind:    apperr.INTERNAL_ERROR,
			Message: "erro inesperado",
			Cause:   err,
		}
	}
}

func mapRepoError(op string, err error) error {
	if err == nil {
		return nil
	}

	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr != nil {
		return appErr
	}

	switch {
	case errors.Is(err, ErrAccountAlreadyExists):
		return &apperr.AppError{
			Kind:    apperr.RESOURCE_ALREADY_EXISTS,
			Message: "usuário já cadastrado",
			Cause:   err,
		}

	case errors.Is(err, ErrAccountNotFound):
		return &apperr.AppError{
			Kind:    apperr.NOT_FOUND,
			Message: "usuário não encontrado",
			Cause:   err,
		}

	case errors.Is(err, persistence.ErrPersistenceFailure):
		return &apperr.AppError{
			Kind:    apperr.INFRA_DATABASE_ERROR,
			Message: "falha técnica",
			Cause:   fmt.Errorf("%s: %w", op, err),
		}

	default:
		return &apperr.AppError{
			Kind:    apperr.INTERNAL_ERROR,
			Message: "erro inesperado",
			Cause:   fmt.Errorf("%s: %w", op, err),
		}
	}
}

func accountNotFound() error {
	return &apperr.AppError{
		Kind:    apperr.NOT_FOUND,
		Message: "usuário não encontrado",
	}
}
