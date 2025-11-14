package patient

import (
	"fmt"
	"net/http"
	"sonnda-api/internal/core/jwt"

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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_input", "details": err.Error()})
		return
	}

	claims, ok := ctx.MustGet("claims").(*jwt.Claims)

	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	SubjectID := claims.ID
	patient, err := h.svc.CreatePatientAsPatient(ctx, SubjectID, input)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, patient)
}

func (h *Handler) Update(ctx *gin.Context) {
	var input UpdatePatientInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_input", "details": err.Error()})
		return
	}

	userID := parseUintParam(ctx, "id")
	actorID := getUserID(ctx)
	actorRole := getUserRole(ctx)

	patient, err := h.svc.Update(ctx, actorID, actorRole, userID, input)
	if err != nil {
		handleServiceError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, patient)
}

func (h *Handler) SelfUpdate(ctx *gin.Context) {
	var input SelfUpdateInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_input", "details": err.Error()})
		return
	}

	userID := getUserID(ctx)

	patient, err := h.svc.SelfUpdate(ctx, userID, input)
	if err != nil {
		handleServiceError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, patient)
}

func (h *Handler) GetByID(ctx *gin.Context) {
	userID := parseUintParam(ctx, "id")
	actorID := getUserID(ctx)
	actorRole := getUserRole(ctx)

	patient, err := h.svc.GetByUserID(ctx, actorID, actorRole, userID)
	if err != nil {
		handleServiceError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, patient)
}

func (h *Handler) List(ctx *gin.Context) {
	// Poderia usar query param pra paginação
	patients, err := h.svc.List(ctx, 100, 0)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_list_patients"})
		return
	}
	ctx.JSON(http.StatusOK, patients)
}

// Helpers
func getUserID(ctx *gin.Context) uint {
	uid, _ := ctx.Get("userID")
	return uid.(uint)
}

func getUserRole(ctx *gin.Context) string {
	role, _ := ctx.Get("role")
	return role.(string)
}

func parseUintParam(ctx *gin.Context, param string) uint {
	id, _ := ctx.Params.Get(param)
	var uid uint
	fmt.Sscanf(id, "%d", &uid)
	return uid
}

func handleServiceError(ctx *gin.Context, err error) {
	switch err {
	case ErrUnauthorizedAccess:
		ctx.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
	case ErrPatientEditRestricted:
		ctx.JSON(http.StatusForbidden, gin.H{"error": "cannot_edit_other_profile"})
	case ErrCPFAlreadyExists:
		ctx.JSON(http.StatusConflict, gin.H{"error": "cpf_already_exists"})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
	}
}
