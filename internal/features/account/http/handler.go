// internal/features/account/http/handler.go
package accounthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
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

const accountCacheControl = "private, no-store"

type accountService interface {
	Update(ctx context.Context, input account.AccountUpdateInput) (*accountdomain.Account, error)
	DeactivateByIdentity(ctx context.Context, issuer, subject string) error
}

type professionalActivator interface {
	Activate(ctx context.Context, accountID uuid.UUID, password, origin string) (*accountdomain.Account, error)
}

type Handler struct {
	accountSvc accountService
	activation professionalActivator
}

type nullableField[T any] struct {
	Set   bool
	Value *T
}

func (f *nullableField[T]) UnmarshalJSON(data []byte) error {
	f.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		f.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	f.Value = &value
	return nil
}

func (f nullableField[T]) Schema(registry huma.Registry) *huma.Schema {
	schema := registry.Schema(reflect.TypeFor[T](), true, "")
	schema.Nullable = true
	return schema
}

type updateAccountInput struct {
	Body updateAccountRequest
}

type updateAccountRequest struct {
	FullName  nullableField[string] `json:"full_name,omitempty" doc:"Nome completo; null remove o valor" minLength:"2" maxLength:"120"`
	BirthDate nullableField[string] `json:"birth_date,omitempty" doc:"Data de nascimento; null remove o valor" format:"date"`
	CPF       nullableField[string] `json:"cpf,omitempty" doc:"CPF sem pontuação; vazio ou null remove o valor" pattern:"^(?:$|[0-9]{11})$"`
	Phone     nullableField[string] `json:"phone,omitempty" doc:"Telefone; vazio ou null remove o valor" pattern:"^(?:$|\\+?[0-9]{10,15})$"`
}

type professionalActivationInput struct {
	Body professionalActivationRequest
}

type professionalActivationRequest struct {
	Password string `json:"password" doc:"Senha de habilitação profissional" minLength:"1"`
}

type accountProfileResponse struct {
	FullName  *string `json:"full_name"`
	BirthDate *string `json:"birth_date" format:"date"`
	CPF       *string `json:"cpf"`
	Phone     *string `json:"phone"`
}

type accountResponse struct {
	ID                  uuid.UUID              `json:"id" format:"uuid"`
	Email               *string                `json:"email" format:"email"`
	AccountType         string                 `json:"account_type"`
	Profile             accountProfileResponse `json:"profile"`
	OnboardingCompleted bool                   `json:"onboarding_completed"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

type accountOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         accountResponse
}

func NewHandler(accountSvc accountService, activation professionalActivator) *Handler {
	return &Handler{accountSvc: accountSvc, activation: activation}
}

// RegisterOnboardedRoutes registers account operations that require completed onboarding.
func (h *Handler) RegisterOnboardedRoutes(onboarded huma.API, security []map[string][]string) {
	huma.Register(onboarded, huma.Operation{
		OperationID: "activateCurrentAccountAsProfessional",
		Method:      http.MethodPost,
		Path:        "/me/professional-activation",
		Summary:     "Habilitar a conta autenticada como profissional",
		Tags:        []string{"Account"},
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError},
		Security:    security,
	}, h.activateCurrentAccountAsProfessional)
}

// RegisterResolvedRoutes registers account operations that require a resolved account.
func (h *Handler) RegisterResolvedRoutes(resolved huma.API, security []map[string][]string) {
	huma.Register(resolved, huma.Operation{
		OperationID: "getCurrentAccount",
		Method:      http.MethodGet,
		Path:        "/me",
		Summary:     "Obter a conta autenticada",
		Tags:        []string{"Account"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
		Security:    security,
	}, h.getCurrentAccount)

	huma.Register(resolved, huma.Operation{
		OperationID: "updateCurrentAccount",
		Method:      http.MethodPatch,
		Path:        "/me",
		Summary:     "Atualizar o perfil da conta autenticada",
		Tags:        []string{"Account"},
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError},
		Security:    security,
	}, h.updateCurrentAccount)
}

// RegisterAuthenticatedRoutes registers account operations that only require authentication.
func (h *Handler) RegisterAuthenticatedRoutes(authenticated huma.API, security []map[string][]string) {
	huma.Register(authenticated, huma.Operation{
		OperationID:   "deleteCurrentAccount",
		Method:        http.MethodDelete,
		Path:          "/me",
		Summary:       "Desativar a conta autenticada",
		Tags:          []string{"Account"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
		Security:      security,
	}, h.deleteCurrentAccount)
}

func (h *Handler) activateCurrentAccountAsProfessional(ctx context.Context, input *professionalActivationInput) (*accountOutput, error) {
	currentAccount, err := currentAccountFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if h == nil || h.activation == nil {
		return nil, humaerror.From(apperr.Internal("habilitação profissional indisponível", errors.New("professional activation service is not configured")))
	}
	activated, err := h.activation.Activate(ctx, currentAccount.ID, input.Body.Password, helpers.GetClientOriginFromContext(ctx))
	if err != nil {
		return nil, humaerror.From(err)
	}
	return accountOutputFromDomain(ctx, activated), nil
}

func (h *Handler) getCurrentAccount(ctx context.Context, _ *struct{}) (*accountOutput, error) {
	currentAccount, err := currentAccountFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return accountOutputFromDomain(ctx, currentAccount), nil
}

func (h *Handler) updateCurrentAccount(ctx context.Context, input *updateAccountInput) (*accountOutput, error) {
	currentAccount, err := currentAccountFromContext(ctx)
	if err != nil {
		return nil, err
	}

	update := account.AccountUpdateInput{
		AccountID: currentAccount.ID,
		FullName:  optionalString(input.Body.FullName),
		CPF:       optionalString(input.Body.CPF),
		Phone:     optionalString(input.Body.Phone),
	}
	update.BirthDate, err = optionalDate(input.Body.BirthDate)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("data de nascimento inválida")
	}

	updated, err := h.accountSvc.Update(ctx, update)
	if err != nil {
		return nil, humaerror.From(err)
	}
	return accountOutputFromDomain(ctx, updated), nil
}

func (h *Handler) deleteCurrentAccount(ctx context.Context, _ *struct{}) (*struct{}, error) {
	identity, ok := authhttp.GetIdentityFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Unauthorized("autenticação necessária"))
	}
	if err := h.accountSvc.DeactivateByIdentity(ctx, identity.Issuer, identity.Subject); err != nil {
		return nil, humaerror.From(err)
	}
	return &struct{}{}, nil
}

func currentAccountFromContext(ctx context.Context) (*accountdomain.Account, error) {
	currentAccount, ok := helpers.GetCurrentAccountFromContext(ctx)
	if !ok {
		return nil, humaerror.From(apperr.Internal("falha ao resolver conta", errors.New("current account is missing from request context")))
	}
	return currentAccount, nil
}

func optionalString(field nullableField[string]) account.OptionalField[string] {
	return account.OptionalField[string]{Set: field.Set, Value: field.Value}
}

func optionalDate(field nullableField[string]) (account.OptionalField[time.Time], error) {
	result := account.OptionalField[time.Time]{Set: field.Set}
	if !field.Set || field.Value == nil {
		return result, nil
	}
	value, err := time.Parse(time.DateOnly, *field.Value)
	if err != nil {
		return account.OptionalField[time.Time]{}, err
	}
	result.Value = &value
	return result, nil
}

func accountOutputFromDomain(ctx context.Context, currentAccount *accountdomain.Account) *accountOutput {
	return &accountOutput{
		CacheControl: accountCacheControl,
		Body:         accountResponseFromDomain(ctx, currentAccount),
	}
}

func accountResponseFromDomain(ctx context.Context, currentAccount *accountdomain.Account) accountResponse {
	var birthDate *string
	if currentAccount.Profile.BirthDate != nil {
		value := currentAccount.Profile.BirthDate.Format(time.DateOnly)
		birthDate = &value
	}
	response := accountResponse{
		ID:          currentAccount.ID,
		AccountType: string(currentAccount.AccountType),
		Profile: accountProfileResponse{
			FullName:  currentAccount.Profile.FullName,
			BirthDate: birthDate,
			CPF:       currentAccount.Profile.CPF,
			Phone:     currentAccount.Profile.Phone,
		},
		OnboardingCompleted: currentAccount.OnboardingCompleted(),
		CreatedAt:           currentAccount.CreatedAt,
		UpdatedAt:           currentAccount.UpdatedAt,
	}
	if identity, ok := authhttp.GetIdentityFromContext(ctx); ok {
		response.Email = identity.Email
	}
	return response
}
