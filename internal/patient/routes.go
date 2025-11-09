package patient

import (
	"sonnda-api/internal/database"
	"sonnda-api/internal/middleware"
	"sonnda-api/internal/user"

	"github.com/gin-gonic/gin"
)

func Routes(rg *gin.RouterGroup) {
	repo := NewRepository(database.DB)
	svc := NewService(repo)
	handler := NewHandler(svc)

	patients := rg.Group("/patients")
	{
		// públicas
		patients.POST("/register", handler.Register)

		protected := patients.Group("")
		protected.Use(middleware.JWTAuthMiddleware())
		protected.Use(middleware.RequireRole(user.RolePatient))

		// protegida
		protected.GET("/me", middleware.JWTAuthMiddleware(), handler.Me)
	}
}
