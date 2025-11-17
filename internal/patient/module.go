package patient

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *Handler
}

func NewModule(db *pgxpool.Pool) *Module {
	repo := NewRepository(db)
	svc := NewService(repo)
	handler := NewHandler(svc)
	return &Module{
		handler: handler,
	}
}

func (m *Module) SetupRoutes(rg *gin.RouterGroup) {
	Routes(rg, m.handler)
}
