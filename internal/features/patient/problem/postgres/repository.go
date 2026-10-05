// internal/features/patient/problem/postgres/repository.go
package postgres

import (
	problemrepository "github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	problemsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/problem"
)

// Repository is the PostgreSQL adapter for patient problems.
type Repository struct {
	client  *postgress.Client
	queries *problemsqlc.Queries
}

var _ problemrepository.Repository = (*Repository)(nil)

func NewRepository(client *postgress.Client) *Repository {
	return &Repository{
		client:  client,
		queries: problemsqlc.New(client.Pool()),
	}
}
