// internal/features/patient/problem/postgres/errors.go
package postgres

import (
	"errors"
	"fmt"

	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
)

func persistenceError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(persistence.ErrPersistenceFailure, fmt.Errorf("%s: %w", operation, err))
}
