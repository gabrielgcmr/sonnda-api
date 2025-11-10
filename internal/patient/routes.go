package patient

import (
	"sonnda-api/internal/database"

	"github.com/gin-gonic/gin"
)

func Routes(rg *gin.RouterGroup) {
	r := NewRepository(database.DB)
	s := NewService(r)
	h := NewHandler(s)

	patients := rg.Group("/patients")
	{
		patients.POST("", h.Create)
		patients.PUT("/:id", h.Update)
		patients.PATCH("/me", h.SelfUpdate)
		patients.GET("/:id", h.GetByID)
		patients.GET("", h.List)
	}
}
