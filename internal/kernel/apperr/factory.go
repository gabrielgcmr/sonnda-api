// internal/kernel/apperr/factory.go
package apperr

// Fábricas de Erro (Factories)
// Validation retorna um erro deBadRequest (400)
func Validation(msg string, violations ...Violation) *AppError {
	return &AppError{
		Kind:       VALIDATION_FAILED,
		Message:    msg,
		Violations: violations,
	}
}

// Internal retorna um erro de servidor (500) e oculta detalhes do cliente
func Internal(msg string, cause error) *AppError {
	return &AppError{
		Kind:    INTERNAL_ERROR,
		Message: msg,
		Cause:   cause,
	}
}

// Conflict para recursos duplicados (409)
func Conflict(msg string) *AppError {
	return &AppError{
		Kind:    RESOURCE_CONFLICT,
		Message: msg,
	}
}

func AlreadyExists(msg string) *AppError {
	return &AppError{
		Kind:    RESOURCE_ALREADY_EXISTS,
		Message: msg,
	}
}

// Unauthorized para falhas de login (401)
func Unauthorized(msg string) *AppError {
	return &AppError{
		Kind:    AUTH_REQUIRED,
		Message: msg,
	}
}

// Forbidden para falhas de permissão (403)
func Forbidden(msg string) *AppError {
	return &AppError{
		Kind:    ACCESS_DENIED,
		Message: msg,
	}
}

func RateLimited(msg string) *AppError {
	return &AppError{
		Kind:    RATE_LIMIT_EXCEEDED,
		Message: msg,
	}
}

// NotFound para recursos não encontrados (404)
func NotFound(msg string) *AppError {
	return &AppError{
		Kind:    NOT_FOUND,
		Message: msg,
	}
}

// DomainRuleViolation retorna um erro de regra de domínio (422)
func DomainRuleViolation(msg string, violations ...Violation) *AppError {
	return &AppError{
		Kind:       DOMAIN_RULE_VIOLATION,
		Message:    msg,
		Violations: violations,
	}
}

// ProfileNotFound identifies an authenticated identity without an application profile.
func ProfileNotFound() *AppError {
	return &AppError{Kind: PROFILE_NOT_FOUND, Message: "cadastro necessário"}
}
