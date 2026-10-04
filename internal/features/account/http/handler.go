// internal/features/account/http/handler.go
package accounthttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type userService interface {
	Update(ctx context.Context, input account.UserUpdateInput) (*accountdomain.User, error)
	Delete(ctx context.Context, userID uuid.UUID) error
}

type professionalActivator interface {
	Activate(ctx context.Context, accountID uuid.UUID, password, origin string) (*accountdomain.User, error)
}

type Handler struct {
	onboarding account.Onboarding
	userSvc    userService
	activation professionalActivator
}

type createAccountInput struct {
	Body createAccountRequest
}

type createAccountRequest struct {
	FullName  string `json:"full_name" doc:"Nome completo" minLength:"2" maxLength:"120"`
	BirthDate string `json:"birth_date" doc:"Data de nascimento" format:"date"`
	CPF       string `json:"cpf" doc:"CPF sem pontuação" pattern:"^[0-9]{11}$"`
	Phone     string `json:"phone" doc:"Telefone" pattern:"^\\+?[0-9]{10,15}$"`
}

type updateAccountInput struct {
	Body updateAccountRequest
}

type updateAccountRequest struct {
	FullName  *string `json:"full_name,omitempty" doc:"Nome completo" minLength:"2" maxLength:"120"`
	BirthDate *string `json:"birth_date,omitempty" doc:"Data de nascimento" format:"date"`
	CPF       *string `json:"cpf,omitempty" doc:"CPF sem pontuação" pattern:"^[0-9]{11}$"`
	Phone     *string `json:"phone,omitempty" doc:"Telefone" pattern:"^\\+?[0-9]{10,15}$"`
}

type professionalActivationInput struct {
	Body professionalActivationRequest
}

type professionalActivationRequest struct {
	Password string `json:"password" doc:"Senha de habilitação profissional" minLength:"1"`
}

type accountUserResponse struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	AuthIssuer  string    `json:"auth_issuer"`
	AuthSubject string    `json:"auth_subject"`
	Email       string    `json:"email" format:"email"`
	FullName    string    `json:"full_name"`
	AccountType string    `json:"account_type"`
	BirthDate   string    `json:"birth_date" format:"date"`
	CPF         string    `json:"cpf"`
	Phone       string    `json:"phone"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type accountUserOutput struct {
	Body accountUserResponse
}

func NewHandler(onboarding account.Onboarding, userSvc userService, activation professionalActivator) *Handler {
	return &Handler{onboarding: onboarding, userSvc: userSvc, activation: activation}
}

// RegisterHumaRoutes registers all account operations into the application's
// single Huma API. The groups define authentication, not a URL version.
func (h *Handler) RegisterHumaRoutes(authenticated huma.API, registered huma.API, security []map[string][]string) {
	huma.Register(authenticated, huma.Operation{
		OperationID:   "createCurrentAccount",
		Method:        http.MethodPost,
		Path:          "/me",
		Summary:       "Criar perfil da conta autenticada",
		Tags:          []string{"Account"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusConflict, http.StatusUnprocessableEntity},
		Security:      security,
	}, h.createCurrentAccount)

	huma.Register(registered, huma.Operation{
		OperationID: "activateCurrentAccountAsProfessional",
		Method:      http.MethodPost,
		Path:        "/me/professional-activation",
		Summary:     "Habilitar a conta autenticada como profissional",
		Tags:        []string{"Account"},
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError},
		Security:    security,
	}, h.activateCurrentAccountAsProfessional)

	huma.Register(registered, huma.Operation{
		OperationID: "getCurrentAccount",
		Method:      http.MethodGet,
		Path:        "/me",
		Summary:     "Obter perfil da conta autenticada",
		Tags:        []string{"Account"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
		Security:    security,
	}, h.getCurrentAccount)

	huma.Register(registered, huma.Operation{
		OperationID: "updateCurrentAccount",
		Method:      http.MethodPut,
		Path:        "/me",
		Summary:     "Atualizar perfil da conta autenticada",
		Tags:        []string{"Account"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
		Security:    security,
	}, h.updateCurrentAccount)

	huma.Register(registered, huma.Operation{
		OperationID:   "deleteCurrentAccount",
		Method:        http.MethodDelete,
		Path:          "/me",
		Summary:       "Excluir perfil da conta autenticada",
		Tags:          []string{"Account"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden},
		Security:      security,
	}, h.deleteCurrentAccount)
}

func (h *Handler) activateCurrentAccountAsProfessional(ctx context.Context, input *professionalActivationInput) (*accountUserOutput, error) {
	currentUser, ok := helpers.GetCurrentUserFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	if h == nil || h.activation == nil {
		return nil, humaerror.From(apperr.Internal("habilitação profissional indisponível", errors.New("professional activation service is not configured")))
	}
	activated, err := h.activation.Activate(ctx, currentUser.ID, input.Body.Password, helpers.GetClientOriginFromContext(ctx))
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &accountUserOutput{Body: accountUserResponseFromDomain(activated)}, nil
}

func (h *Handler) createCurrentAccount(ctx context.Context, input *createAccountInput) (*accountUserOutput, error) {
	identity, ok := authhttp.GetIdentityFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("autenticação necessária")
	}
	if identity.Email == nil || strings.TrimSpace(*identity.Email) == "" {
		return nil, huma.Error422UnprocessableEntity("email é obrigatório")
	}

	birthDate, err := time.Parse(time.DateOnly, input.Body.BirthDate)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("data de nascimento inválida")
	}

	created, err := h.onboarding.Register(ctx, account.RegisterInput{
		Issuer:      identity.Issuer,
		Subject:     identity.Subject,
		Email:       strings.TrimSpace(*identity.Email),
		AccountType: accountdomain.AccountTypeBasicCare,
		FullName:    input.Body.FullName,
		BirthDate:   birthDate,
		CPF:         input.Body.CPF,
		Phone:       input.Body.Phone,
	})
	if err != nil {
		return nil, humaerror.From(err)
	}

	return &accountUserOutput{Body: accountUserResponseFromDomain(created)}, nil
}

func (h *Handler) getCurrentAccount(ctx context.Context, _ *struct{}) (*accountUserOutput, error) {
	currentUser, ok := helpers.GetCurrentUserFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	return &accountUserOutput{Body: accountUserResponseFromDomain(currentUser)}, nil
}

func (h *Handler) updateCurrentAccount(ctx context.Context, input *updateAccountInput) (*accountUserOutput, error) {
	currentUser, ok := helpers.GetCurrentUserFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}

	update := account.UserUpdateInput{
		UserID:   currentUser.ID,
		FullName: input.Body.FullName,
		CPF:      input.Body.CPF,
		Phone:    input.Body.Phone,
	}
	if input.Body.BirthDate != nil {
		birthDate, err := time.Parse(time.DateOnly, *input.Body.BirthDate)
		if err != nil {
			return nil, huma.Error422UnprocessableEntity("data de nascimento inválida")
		}
		update.BirthDate = &birthDate
	}

	updated, err := h.userSvc.Update(ctx, update)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &accountUserOutput{Body: accountUserResponseFromDomain(updated)}, nil
}

func (h *Handler) deleteCurrentAccount(ctx context.Context, _ *struct{}) (*struct{}, error) {
	currentUser, ok := helpers.GetCurrentUserFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	if err := h.userSvc.Delete(ctx, currentUser.ID); err != nil {
		return nil, humaerror.From(err)
	}
	return &struct{}{}, nil
}

func accountUserResponseFromDomain(user *accountdomain.User) accountUserResponse {
	return accountUserResponse{
		ID:          user.ID,
		AuthIssuer:  user.AuthIssuer,
		AuthSubject: user.AuthSubject,
		Email:       user.Email,
		FullName:    user.FullName,
		AccountType: string(user.AccountType),
		BirthDate:   user.BirthDate.Format(time.DateOnly),
		CPF:         user.CPF,
		Phone:       user.Phone,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}
