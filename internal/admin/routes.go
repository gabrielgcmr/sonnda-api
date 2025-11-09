package admin

import (
	"sonnda-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

func Routes(rg *gin.RouterGroup) {
	//repo := NewRepository(database.DB)
	//svc := NewService(repo)
	//handler := NewHandler(svc)

	// Todas as rotas de admin requerem autenticação e role de admin
	admin := rg.Group("/admin")
	admin.Use(middleware.JWTAuthMiddleware())
	admin.Use(middleware.RequireAdmin())
	{
		// Gestão de usuários
		// GET /api/v1/admin/users
		//admin.GET("/users", handler.ListUsers)

		// GET /api/v1/admin/users/:id
		//admin.GET("/users/:id", handler.GetUser)

		// POST /api/v1/admin/users
		//admin.POST("/users", handler.CreateUser)

		// PUT /api/v1/admin/users/:id
		//admin.PUT("/users/:id", handler.UpdateUser)

		// DELETE /api/v1/admin/users/:id
		////admin.DELETE("/users/:id", handler.DeleteUser)

		// Dashboard e relatórios
		// GET /api/v1/admin/dashboard
		//admin.GET("/dashboard", handler.Dashboard)

		// GET /api/v1/admin/reports
		//admin.GET("/reports", handler.Reports)

		// Configurações do sistema
		// GET /api/v1/admin/settings
		//admin.GET("/settings", handler.GetSettings)

		// PUT /api/v1/admin/settings
		//admin.PUT("/settings", handler.UpdateSettings)
	}
}
