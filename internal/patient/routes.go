package patient

import (
	"github.com/gabrielgcmr/sonnda-api/internal/database"
	"github.com/gabrielgcmr/sonnda-api/internal/middleware"
	"github.com/gin-gonic/gin"
)

func Routes(r *gin.Engine) {
	repo := NewRepository(database.DB)
	svc := NewService(repo)
	handler := NewHandler(svc)

	grp := r.Group("/api/patients")
	{
		// públicas
		grp.POST("/register", handler.Register)
		grp.POST("/login", handler.Login)

		// protegida
		grp.GET("/me", middleware.JWTAuthMiddleware(), handler.Me)
	}
}
