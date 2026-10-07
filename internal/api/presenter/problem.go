// internal/api/presenter/problem.go
package presenter

import (
	"net/http"
	"strings"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type ErrorCode = apperr.ErrorKind
type Violation = apperr.Violation

// Problem representa um erro no formato RFC 9457.
type Problem struct {
	// Campos obrigatórios RFC 9457
	Type     string `json:"type,omitempty"`     // URI que identifica o tipo de problema
	Title    string `json:"title"`              // Título breve do problema
	Status   int    `json:"status"`             // Código HTTP
	Detail   string `json:"detail,omitempty"`   // Descrição detalhada
	Instance string `json:"instance,omitempty"` // URI específica da ocorrência

	// Extensões RFC 9457 (opcionais)
	Code       ErrorCode   `json:"code,omitempty"`       // Código estável do erro (contrato Sonnda)
	Violations []Violation `json:"violations,omitempty"` // Validações
	TraceID    string      `json:"traceId,omitempty"`    // ID para rastreamento (normalmente X-Request-ID)
	Timestamp  time.Time   `json:"timestamp,omitempty"`  // Quando ocorreu

	// Campo interno para causa/original
	cause error `json:"-"`
}

type ProblemMeta struct {
	Instance string
	TraceID  string
}

func NewProblem(status int, code ErrorCode, detail string, violations []Violation, meta ProblemMeta, cause error) Problem {
	return Problem{
		Type:       ProblemTypeFromCode(code),
		Title:      ProblemTitleFromCode(code, status),
		Status:     status,
		Detail:     detail,
		Instance:   strings.TrimSpace(meta.Instance),
		Code:       code,
		Violations: violations,
		TraceID:    strings.TrimSpace(meta.TraceID),
		Timestamp:  time.Now().UTC(),
		cause:      cause,
	}
}

func ProblemTypeFromCode(code ErrorCode) string {
	c := strings.TrimSpace(string(code))
	if c == "" {
		return "about:blank"
	}
	// URN estável e válido como URI.
	return "urn:sonnda:problem:" + strings.ToLower(c)
}

func ProblemTitleFromCode(code ErrorCode, status int) string {
	switch code {
	// AUTH
	case apperr.AUTH_REQUIRED, apperr.AUTH_TOKEN_INVALID, apperr.AUTH_TOKEN_EXPIRED:
		return "Não autorizado"

	// ACCESS AND PERMISSIONS
	case apperr.PROFILE_NOT_FOUND:
		return "Cadastro necessário"
	case apperr.ACCESS_DENIED, apperr.ACTION_NOT_ALLOWED, apperr.ONBOARDING_REQUIRED, apperr.ACCOUNT_DEACTIVATED:
		return "Acesso negado"

	// VALIDATION
	case apperr.VALIDATION_FAILED,
		apperr.REQUIRED_FIELD_MISSING,
		apperr.INVALID_FIELD_FORMAT,
		apperr.INVALID_ENUM_VALUE,
		apperr.INVALID_DATE:
		return "Falha de validação"

	// NOT FOUND
	case apperr.NOT_FOUND:
		return "Não encontrado"

	// CONFLICT
	case apperr.RESOURCE_CONFLICT, apperr.RESOURCE_ALREADY_EXISTS:
		return "Conflito"

	// DOMAIN
	case apperr.DOMAIN_RULE_VIOLATION:
		return "Regra de negócio violada"

	// RATE
	case apperr.RATE_LIMIT_EXCEEDED:
		return "Muitas requisições"
	case apperr.UPLOAD_SIZE_EXCEEDED:
		return "Payload muito grande"

	// INFRA / INTERNAL
	case apperr.INFRA_EXTERNAL_SERVICE_ERROR,
		apperr.INFRA_TIMEOUT,
		apperr.INFRA_AUTHENTICATION_ERROR,
		apperr.INFRA_DATABASE_ERROR,
		apperr.INFRA_STORAGE_ERROR,
		apperr.INTERNAL_ERROR:
		return "Erro interno"
	}

	if t := strings.TrimSpace(http.StatusText(status)); t != "" {
		return t
	}
	return "Erro"
}
