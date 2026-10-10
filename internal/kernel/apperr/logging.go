// internal/kernel/apperr/logging.go
package apperr

import (
	"errors"
	"log/slog"
)

func ErrorCodeOf(err error) ErrorKind {
	var ae *AppError
	if errors.As(err, &ae) && ae != nil {
		return ae.Kind
	}
	return INTERNAL_ERROR
}

// LogLevelOf decide dinamicamente o nível do log baseado no código do erro
func LogLevelOf(err error) slog.Level {
	code := ErrorCodeOf(err)

	switch code {
	// 5xx – erro do sistema / infra
	case INTERNAL_ERROR,
		INFRA_AUTHENTICATION_ERROR,
		INFRA_DATABASE_ERROR,
		INFRA_STORAGE_ERROR,
		INFRA_EXTERNAL_SERVICE_ERROR,
		INFRA_TIMEOUT:
		return slog.LevelError

	// Limite / abuso / payload problemático
	case RATE_LIMIT_EXCEEDED,
		UPLOAD_SIZE_EXCEEDED:
		return slog.LevelWarn

	// Tudo que é erro esperado de cliente → Info
	case AUTH_REQUIRED,
		AUTH_TOKEN_INVALID,
		AUTH_TOKEN_EXPIRED,
		ACCESS_DENIED,
		PROFILE_NOT_FOUND,
		ACTION_NOT_ALLOWED,
		ONBOARDING_REQUIRED,
		ACCOUNT_DEACTIVATED,
		VALIDATION_FAILED,
		NOT_FOUND,
		RESOURCE_CONFLICT,
		RESOURCE_ALREADY_EXISTS,
		DOMAIN_RULE_VIOLATION:
		return slog.LevelInfo

	// Fallback seguro
	default:
		return slog.LevelError
	}
}
