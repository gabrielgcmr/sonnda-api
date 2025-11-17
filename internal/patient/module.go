package patient

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

func Build(db *pgxpool.Pool) *Handler {
	repo := NewRepository(db)
	svc := NewService(repo)
	return NewHandler(svc)
}
