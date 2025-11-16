package exam

import "github.com/gin-gonic/gin"

func Routes(r *gin.RouterGroup, h *Handler) {
	exam := r.Group("/exam")
	exam.POST("/upload", h.Upload)
}
