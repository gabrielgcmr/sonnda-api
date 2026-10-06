// internal/features/account/http/handler.go
package accounthttp

import (
	"context"
	"errors"
	"net/http"
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

type accountService interface {
	Update(ctx context.Context, input account.AccountUpdateInput) (*accountdomain.Account, error)
	SoftDelete(ctx context.Context, accountID uuid.UUID) error
}

type professionalActivator interface {
	Activate(ctx context.Context, accountID uuid.UUID, password, origin string) (*accountdomain.Account, error)
}

type Handler struct {
	onboarding account.Onboarding
	accountSvc accountService
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

type accountResponse struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	AuthIssuer  string    `json:"auth_issuer"`
	AuthSubject string    `json:"auth_subject"`
	Email       *string   `json:"email" format:"email"`
	FullName    *string   `json:"full_name"`
	AccountType string    `json:"account_type"`
	BirthDate   *string   `json:"birth_date" format:"date"`
	CPF         *string   `json:"cpf"`
	Phone       *string   `json:"phone"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type accountOutput struct {
	Body accountResponse
}

func NewHandler(onboarding account.Onboarding, accountSvc accountService, activation professionalActivator) *Handler {
	return &Handler{onboarding: onboarding, accountSvc: accountSvc, activation: activation}
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

func (h *Handler) activateCurrentAccountAsProfessional(ctx context.Context, input *professionalActivationInput) (*accountOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	if h == nil || h.activation == nil {
		return nil, humaerror.From(apperr.Internal("habilitação profissional indisponível", errors.New("professional activation service is not configured")))
	}
	activated, err := h.activation.Activate(ctx, currentAccount.ID, input.Body.Password, helpers.GetClientOriginFromContext(ctx))
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &accountOutput{Body: accountResponseFromDomain(ctx, activated)}, nil
}

func (h *Handler) createCurrentAccount(ctx context.Context, input *createAccountInput) (*accountOutput, error) {
	identity, ok := authhttp.GetIdentityFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("autenticação necessária")
	}

	birthDate, err := time.Parse(time.DateOnly, input.Body.BirthDate)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("data de nascimento inválida")
	}

	created, err := h.onboarding.Register(ctx, account.RegisterInput{
		Issuer:      identity.Issuer,
		Subject:     identity.Subject,
		Email:       identity.Email,
		AccountType: accountdomain.AccountTypeBasicCare,
		Profile: accountdomain.Profile{
			FullName:  &input.Body.FullName,
			BirthDate: &birthDate,
			CPF:       &input.Body.CPF,
			Phone:     &input.Body.Phone,
		},
	})
	if err != nil {
		return nil, humaerror.From(err)
	}

	return &accountOutput{Body: accountResponseFromDomain(ctx, created)}, nil
}

func (h *Handler) getCurrentAccount(ctx context.Context, _ *struct{}) (*accountOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	return &accountOutput{Body: accountResponseFromDomain(ctx, currentAccount)}, nil
}

func (h *Handler) updateCurrentAccount(ctx context.Context, input *updateAccountInput) (*accountOutput, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}

	update := account.AccountUpdateInput{
		AccountID: currentAccount.ID,
		FullName:  input.Body.FullName,
		CPF:       input.Body.CPF,
		Phone:     input.Body.Phone,
	}
	if input.Body.BirthDate != nil {
		birthDate, err := time.Parse(time.DateOnly, *input.Body.BirthDate)
		if err != nil {
			return nil, huma.Error422UnprocessableEntity("data de nascimento inválida")
		}
		update.BirthDate = &birthDate
	}

	updated, err := h.accountSvc.Update(ctx, update)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return &accountOutput{Body: accountResponseFromDomain(ctx, updated)}, nil
}

func (h *Handler) deleteCurrentAccount(ctx context.Context, _ *struct{}) (*struct{}, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("conta registrada necessária")
	}
	if err := h.accountSvc.SoftDelete(ctx, currentAccount.ID); err != nil {
		return nil, humaerror.From(err)
	}
	return &struct{}{}, nil
}

func accountResponseFromDomain(ctx context.Context, user *accountdomain.Account) accountResponse {
	var birthDate *string
	if user.Profile.BirthDate != nil {
		value := user.Profile.BirthDate.Format(time.DateOnly)
		birthDate = &value
	}
	response := accountResponse{
		ID:          user.ID,
		FullName:    user.Profile.FullName,
		AccountType: string(user.AccountType),
		BirthDate:   birthDate,
		CPF:         user.Profile.CPF,
		Phone:       user.Profile.Phone,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
	if identity, ok := authhttp.GetIdentityFromContext(ctx); ok {
		response.AuthIssuer = identity.Issuer
		response.AuthSubject = identity.Subject
		response.Email = identity.Email
	}
	return response
}
