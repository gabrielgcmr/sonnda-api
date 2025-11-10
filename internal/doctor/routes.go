package doctor

import (
	user "sonnda-api/internal/core/model"
	"sonnda-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

func Routes(rg *gin.RouterGroup) {
	//repo := NewRepository(database.DB)
	//svc := NewService(repo)
	//handler := NewHandler(svc)

	doctors := rg.Group("/doctors")
	{
		// Rotas públicas (listagem para pacientes escolherem médicos)
		// GET /api/v1/doctors
		//doctors.GET("", handler.ListAll)

		// GET /api/v1/doctors/:id
		//doctors.GET("/:id", handler.GetByID)

		// Rotas protegidas - apenas médicos
		protected := doctors.Group("")

		protected.Use(middleware.RequireRole(user.RoleDoctor))
		{
			// GET /api/v1/doctors/me
			//protected.GET("/me", handler.Me)

			// PUT /api/v1/doctors/me
			//protected.PUT("/me", handler.UpdateProfile)

			// GET /api/v1/doctors/patients
			//protected.GET("/patients", handler.ListMyPatients)

			// GET /api/v1/doctors/appointments
			//protected.GET("/appointments", handler.ListMyAppointments)

			// PUT /api/v1/doctors/appointments/:id
			//protected.PUT("/appointments/:id", handler.UpdateAppointment)
		}

		// Rotas apenas para admins
		adminOnly := doctors.Group("")

		adminOnly.Use(middleware.RequireAdmin())
		{
			// POST /api/v1/doctors
			//adminOnly.POST("", handler.Create)

			// PUT /api/v1/doctors/:id
			//adminOnly.PUT("/:id", handler.Update)

			// DELETE /api/v1/doctors/:id
			//adminOnly.DELETE("/:id", handler.Delete)
		}
	}
}
