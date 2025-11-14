package patient

import (
	"github.com/gin-gonic/gin"
)

func Routes(rg *gin.RouterGroup, h *Handler) {
	patients := rg.Group("/patients")

	// Rotas que qualquer usuário autenticado pode usar
	// patients.Use(authMiddleware)

	patients.POST("", h.Create)
	patients.GET("", h.List)

	// Rotas que mexem com identificação individual
	patients.GET("/:id", h.GetByID)
	patients.PUT("/:id", h.Update)
	//patients.PATCH("/:id", h.PartialUpdate)

	// Rotas relativas ao próprio usuário logado
	me := rg.Group("/me")
	// me.Use(authMiddleware)
	//me.GET("", h.SelfGet)
	me.PATCH("", h.SelfUpdate)
}
