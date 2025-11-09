// internal/middleware/rbac.go
package middleware

import (
	"net/http"

	"sonnda-api/internal/database"
	"sonnda-api/internal/user"

	"github.com/gin-gonic/gin"
)

// RequireRole retorna um middleware que verifica se o usuário tem uma das roles permitidas
func RequireRole(allowedRoles ...user.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Obtém o user_id do contexto (definido pelo JWTAuthMiddleware)
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "user not authenticated",
			})
			return
		}

		userID, ok := userIDInterface.(uint)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "invalid user_id format",
			})
			return
		}

		// Busca o usuário no banco de dados
		var usr user.User
		if err := database.DB.First(&usr, userID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "user not found",
			})
			return
		}

		// Verifica se a role do usuário está entre as permitidas
		hasPermission := false
		for _, allowedRole := range allowedRoles {
			if usr.Role == allowedRole {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":         "insufficient permissions",
				"required_role": allowedRoles,
				"user_role":     usr.Role,
			})
			return
		}

		// Armazena a role no contexto para uso posterior
		c.Set("user_role", usr.Role)
		c.Next()
	}
}

// RequireAdmin é um atalho para RequireRole(user.RoleAdmin)
func RequireAdmin() gin.HandlerFunc {
	return RequireRole(user.RoleAdmin)
}

// RequireDoctor é um atalho para RequireRole(user.RoleDoctor)
func RequireDoctor() gin.HandlerFunc {
	return RequireRole(user.RoleDoctor)
}

// RequireDoctorOrAdmin permite acesso para médicos ou admins
func RequireDoctorOrAdmin() gin.HandlerFunc {
	return RequireRole(user.RoleDoctor, user.RoleAdmin)
}

// RequirePatient é um atalho para RequireRole(user.RolePatient)
func RequirePatient() gin.HandlerFunc {
	return RequireRole(user.RolePatient)
}

// GetUserRole retorna a role do usuário autenticado do contexto
func GetUserRole(c *gin.Context) (user.Role, bool) {
	role, exists := c.Get("user_role")
	if !exists {
		return "", false
	}
	userRole, ok := role.(user.Role)
	return userRole, ok
}

// GetUserID retorna o ID do usuário autenticado do contexto
func GetUserID(c *gin.Context) (uint, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	id, ok := userID.(uint)
	return id, ok
}
