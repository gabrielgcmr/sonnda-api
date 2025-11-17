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

func (h *Handler) Create(ctx *gin.Context) {
	var input CreatePatientInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"details": err.Error()})
		return
	}

	patient, err := h.svc.Create(ctx, input)
	if err != nil {
		handleServiceError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, patient)
}

func (h *Handler) UpdateByCPF(ctx *gin.Context) {
	cpf := ctx.Param("cpf")

	var input UpdatePatientInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_input",
			"details": err.Error()})
		return
	}

	patient, err := h.svc.UpdateByCPF(ctx, cpf, input)
	if err != nil {
		handleServiceError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, patient)
}

func (h *Handler) List(ctx *gin.Context) {
	list, err := h.svc.List(ctx, 100, 0)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_list_patients"})
		return
	}

	ctx.JSON(http.StatusOK, list)
}

func handleServiceError(ctx *gin.Context, err error) {
	switch err {
	case ErrCPFAlreadyExists:
		ctx.JSON(http.StatusConflict, gin.H{"error": "cpf_already_exists"})
	case ErrPatientNotFound:
		ctx.JSON(http.StatusNotFound, gin.H{"error": "patient_not_found"})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
	}
}
