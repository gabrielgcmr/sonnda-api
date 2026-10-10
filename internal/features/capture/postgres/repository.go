// internal/features/capture/postgres/repository.go
package capturepostgres

import (
	"context"
	"errors"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	capturesqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/capture"
	"github.com/jackc/pgx/v5"
)

type transactionDB interface {
	capturesqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Repository struct {
	db      transactionDB
	queries capturesqlc.Querier
}

var _ capture.Repository = (*Repository)(nil)

func NewRepository(client *postgress.Client) *Repository {
	if client == nil {
		return &Repository{}
	}
	return newRepository(client.Pool())
}

func newRepository(db transactionDB) *Repository {
	return &Repository{db: db, queries: capturesqlc.New(db)}
}

func (r *Repository) WithinTransaction(ctx context.Context, fn func(capture.Repository) error) error {
	if r == nil || r.db == nil || fn == nil {
		return persistenceError("begin capture transaction", errors.New("capture transaction is not configured"))
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return persistenceError("begin capture transaction", err)
	}
	defer rollback(ctx, tx)
	if err := fn(&Repository{queries: capturesqlc.New(tx)}); err != nil {
		return err
	}
	return persistenceError("commit capture transaction", tx.Commit(ctx))
}

func rollback(ctx context.Context, tx pgx.Tx) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(cleanup)
}
