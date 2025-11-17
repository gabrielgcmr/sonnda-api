package middleware

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var supabaseJWTSecret []byte

// Deve ser chamado no main() antes de registrar as rotas protegidas.
func InitSupabaseAuth() error {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return errors.New("JWT_SECRET não definido no ambiente")
	}
	supabaseJWTSecret = []byte(secret)
	return nil
}

// SupabaseAuth é o middleware que valida o token emitido pelo Supabase.
func SupabaseAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_authorization_header"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_authorization_format"})
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			// Supabase Auth usa HS256 por padrão
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("invalid_signing_method")
			}
			return supabaseJWTSecret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_or_expired_token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token_claims"})
			c.Abort()
			return
		}

		// Supabase coloca o ID do usuário em "sub"
		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_sub_claim"})
			c.Abort()
			return
		}

		// Opcional: pegar email e role
		email, _ := claims["email"].(string)
		role, _ := claims["role"].(string) // 'authenticated', 'anon', etc.

		// Salva no contexto para os handlers
		c.Set("user_id", sub)
		if email != "" {
			c.Set("user_email", email)
		}
		if role != "" {
			c.Set("user_role", role)
		}

		c.Next()
	}
}
