// internal/features/documentprocessing/error.go
package documentprocessing

import (
	"errors"
	"fmt"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

func mapRepoError(op string, err error) error {
	if err == nil {
		return nil
	}

	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr != nil {
		return appErr
	}

	return &apperr.AppError{
		Kind:    apperr.INFRA_DATABASE_ERROR,
		Message: "falha tecnica",
		Cause:   fmt.Errorf("%s: %w", op, err),
	}
}
