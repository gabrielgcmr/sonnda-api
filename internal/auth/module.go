package auth

import (
	"sonnda-api/internal/core/jwt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Module struct {
	Handler *Handler
	jwtMgr  *jwt.JWTManager
}

func NewModule(db *gorm.DB, jwtMgr *jwt.JWTManager) *Module {
	repo := NewRepository(db)
	svc := NewService(repo, jwtMgr)
	handler := NewHandler(svc)

	return &Module{
		Handler: handler,
		jwtMgr:  jwtMgr,
	}
}

func (m *Module) SetupRoutes(rg *gin.RouterGroup) {
	Routes(rg, m.Handler, m.jwtMgr)
}
