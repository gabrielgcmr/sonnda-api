package patient

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler encapsula as rotas de usuário (registro e login).
type Handler struct {
	svc Service
}

// NewHandler cria um novo User handler com o Service injetado.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

type registerRequest struct {
	FullName string  `json:"full_name" binding:"required,min=2"`
	CPF      string  `json:"cpf" binding:"required,len=11"`
	CNS      string  `json:"cns" binding:"required,len=15"`
	Email    *string `json:"email" binding:"omitempty,email"`
	Phone    *string `json:"phone" binding:"omitempty"`
	Password string  `json:"password" binding:"required,min=6"`
}

// Register trata POST /users/register
func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

}

// Login trata POST /users/login

func (h *Handler) Me(c *gin.Context) {

}
