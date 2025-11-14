package auth

import (
	"sonnda-api/internal/core/jwt"

	"gorm.io/gorm"
)

func Build(db *gorm.DB, jwtMgr *jwt.JWTManager) *Handler {
	repo := NewRepository(db)
	svc := NewService(repo, jwtMgr)
	return NewHandler(svc)
}
