package auth

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"sonnda-api/internal/core/user"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

type registerRequest struct {
	Name     string    `json:"name" binding:"required,min=2"`
	Email    string    `json:"email" binding:"required,email"`
	Password string    `json:"password" binding:"required,min=6"`
	Role     user.Role `json:"role" binding:"required"` // futura validação: permitir só alguns
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (handler *Handler) Register(ctx *gin.Context) {
	var req registerRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {

		var verrs validator.ValidationErrors
		if errors.As(err, &verrs) {
			details := make(map[string]string)
			for _, fe := range verrs {
				field := toJSONField(fe.Field()) // Name -> name, etc.
				switch fe.Tag() {
				case "required":
					details[field] = "campo obrigatório"
				case "email":
					details[field] = "formato de e-mail inválido"
				case "min":
					details[field] = "não atende o tamanho mínimo"
				default:
					details[field] = "valor inválido"
				}
			}
			ctx.JSON(http.StatusBadRequest, gin.H{
				"error":   "validation_error",
				"details": details,
			})
			return
		}

		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"details": err.Error(), // debug amigável
		})
		return
	}

	user, err := handler.svc.Register(ctx, req.Name, req.Email, req.Password, req.Role)
	if err != nil {
		if err == ErrEmailTaken {
			ctx.JSON(http.StatusConflict, gin.H{"error": "email_taken"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"id":    user.ID,
		"email": user.Email,
		"role":  user.Role,
	})
}

func (handler *Handler) Login(ctx *gin.Context) {
	var req loginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		var verrs validator.ValidationErrors
		if errors.As(err, &verrs) {
			details := make(map[string]string)
			for _, fe := range verrs {
				field := toJSONField(fe.Field())
				switch fe.Tag() {
				case "required":
					details[field] = "campo obrigatório"
				case "email":
					details[field] = "formato de e-mail inválido"
				default:
					details[field] = "valor inválido"
				}
			}
			ctx.JSON(http.StatusBadRequest, gin.H{
				"error":   "validation_error",
				"details": details,
			})
			return
		}

		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"details": err.Error(),
		})
		return
	}

	user, token, err := handler.svc.Login(ctx, req.Email, req.Password)
	if err != nil {
		if err == ErrInvalidCredentials {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"accessToken": token,
		"user": gin.H{
			"id":    user.ID,
			"email": user.Email,
			"role":  user.Role,
		},
	})
}

func (handler *Handler) Me(ctx *gin.Context) {
	uidVal, ok := ctx.Get("userID")
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	uid, _ := uidVal.(uint)

	u, err := handler.svc.Me(ctx, uid)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"id":    u.ID,
		"email": u.Email,
		"role":  u.Role,
	})
}

func toJSONField(field string) string {
	if field == "" {
		return ""
	}
	// simples: lowercase primeira letra
	return strings.ToLower(field[:1]) + field[1:]
}

// helper opcional se o ID vier como string no contexto:
func parseUserID(raw any) (uint, bool) {
	switch v := raw.(type) {
	case uint:
		return v, true
	case string:
		id, err := strconv.ParseUint(v, 10, 64)
		return uint(id), err == nil
	default:
		return 0, false
	}
}
