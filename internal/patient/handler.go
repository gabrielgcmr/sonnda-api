// internal/user/handler.go
package patient

import (
	"net/http"

	"github.com/gabrielgcmr/sonnda-api/internal/patient/dto"
	"github.com/gabrielgcmr/sonnda-api/internal/patient/utils"
	"github.com/gabrielgcmr/sonnda-api/pkg/validation"
	"github.com/gin-gonic/gin"
)

// Handler encapsula as rotas de usuário (registro e login).
type Handler struct {
	svc *Service
}

// NewHandler cria um novo User handler com o Service injetado.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register trata POST /users/register
func (h *Handler) Register(c *gin.Context) {
	var input dto.RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validation.Validate.Struct(input); err != nil {
		errs := validation.TranslateErrors(err)
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	u, err := h.svc.Register(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := utils.GenerateJWT(uint(u.ID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
		return
	}

	resp := dto.Response{
		ID:        u.ID,
		FullName:  u.FullName,
		CPF:       u.CPF,
		CNS:       u.CNS,
		Email:     *u.Email,
		Phone:     u.Phone,
		CreatedAt: *u.CreatedAt,
		UpdatedAt: *u.UpdatedAt,
		Token:     &token,
	}
	c.JSON(http.StatusCreated, resp)
}

// Login trata POST /users/login
func (h *Handler) Login(c *gin.Context) {
	var input dto.LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validation.Validate.Struct(input); err != nil {
		errs := validation.TranslateErrors(err)
		c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
		return
	}

	u, err := h.svc.Login(input.Email, input.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := utils.GenerateJWT(uint(u.ID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
		return
	}

	resp := dto.Response{
		ID:        u.ID,
		FullName:  u.FullName,
		CPF:       u.CPF,
		CNS:       u.CNS,
		Email:     *u.Email,
		Phone:     u.Phone,
		CreatedAt: *u.CreatedAt,
		UpdatedAt: *u.UpdatedAt,
		Token:     &token,
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Me(c *gin.Context) {
	// 1) Extrai o user_id que o middleware colocou no contexto
	raw, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user_id não encontrado no contexto"})
		return
	}
	userID, ok := raw.(uint)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "formato de user_id inválido"})
		return
	}

	// 2) Busca no service
	u, err := h.svc.GetByID(int(userID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	// 3) Monta o DTO de resposta (sem token, sem senha)
	resp := dto.Response{
		ID:        u.ID,
		FullName:  u.FullName,
		CPF:       u.CPF,
		CNS:       u.CNS,
		Email:     *u.Email,
		Phone:     u.Phone,
		CreatedAt: *u.CreatedAt,
		UpdatedAt: *u.UpdatedAt,
	}
	c.JSON(http.StatusOK, resp)
}
