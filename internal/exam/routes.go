package exam

import "github.com/gin-gonic/gin"

func RegisterRoutes(r *gin.RouterGroup, h *Handler) {
	exam := r.Group("/exam")
	exam.POST("/upload", h.Upload)
}
