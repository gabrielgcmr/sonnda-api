package auth

import (
	"sonnda-api/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func AuthRoutes(rg *gin.RouterGroup, db *gorm.DB, jwt *middleware.JWTManager) {
	repo := NewRepository(db)
	svc := NewService(repo, jwt)
	h := NewHandler(svc)

	// Rotas públicas de autenticação
	// POST /api/v1/auth/login
	rg.POST("/login", h.Login)

	// POST /api/v1/auth/register
	rg.POST("/register", h.Register)

	// POST /api/v1/auth/refresh
	//auth.POST("/refresh", h.RefreshToken)

	// POST /api/v1/auth/logout
	//auth.POST("/logout", h.Logout)

	// POST /api/v1/auth/forgot-password
	//auth.POST("/forgot-password", h.ForgotPassword)

	// POST /api/v1/auth/reset-password
	//auth.POST("/reset-password", h.ResetPassword)

	// Rotas protegidas - requerem autenticação

	protected := rg.Group("")
	protected.Use(NewAuthMiddleware(jwt))
	{
		protected.GET("/me", h.Me)
	}

}
