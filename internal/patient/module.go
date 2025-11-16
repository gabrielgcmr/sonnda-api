package patient

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Module struct {
	handler *Handler
}

func NewModule(db *gorm.DB) *Module {
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
