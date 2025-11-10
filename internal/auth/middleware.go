package auth

import (
	"net/http"
	"strings"

	"sonnda-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

func NewAuthMiddleware(jwt *middleware.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing_token"})
			return
		}
		raw := strings.TrimPrefix(h, "Bearer ")

		claims, err := jwt.Parse(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}
