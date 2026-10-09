// internal/features/capture/postgres/errors.go
package capturepostgres

import (
	"errors"
	"fmt"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func persistenceError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(persistence.ErrPersistenceFailure, fmt.Errorf("%s: %w", operation, err))
}

func writeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23514") {
		return capture.ErrStateConflict
	}
	return persistenceError(operation, err)
}

func resultError(operation string, err error, notFound error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	return writeError(operation, err)
}
