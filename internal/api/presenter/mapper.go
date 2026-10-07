// internal/api/presenter/mapper.go
// internal/api/presenter/mapper.go
package presenter

import (
	"errors"
	"net/http"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

func ToProblem(err error, meta ProblemMeta) (status int, body Problem) {
	var appErr *apperr.AppError

	if errors.As(err, &appErr) && appErr != nil {
		status = StatusFromCode(appErr.Kind)
		return status, NewProblem(status, appErr.Kind, appErr.Message, appErr.Violations, meta, err)
	}

	status = StatusFromCode(apperr.INTERNAL_ERROR)
	return status, NewProblem(status, apperr.INTERNAL_ERROR, "erro inesperado", nil, meta, err)
}

func StatusFromCode(code apperr.ErrorKind) int {
	switch code {
	// AUTH
	case apperr.AUTH_REQUIRED,
		apperr.AUTH_TOKEN_INVALID,
		apperr.AUTH_TOKEN_EXPIRED:
		return http.StatusUnauthorized // 401

	// ACCESS AND PERMISSIONS
	case apperr.PROFILE_NOT_FOUND,
		apperr.ACCESS_DENIED,
		apperr.ACTION_NOT_ALLOWED,
		apperr.ONBOARDING_REQUIRED,
		apperr.ACCOUNT_DEACTIVATED:
		return http.StatusForbidden // 403

	// VALIDATION
	case apperr.VALIDATION_FAILED,
		apperr.REQUIRED_FIELD_MISSING,
		apperr.INVALID_FIELD_FORMAT,
		apperr.INVALID_ENUM_VALUE,
		apperr.INVALID_DATE:
		return http.StatusBadRequest // 400

	// NOT FOUND
	case apperr.NOT_FOUND:
		return http.StatusNotFound // 404

	// CONFLICT
	case apperr.RESOURCE_CONFLICT,
		apperr.RESOURCE_ALREADY_EXISTS:
		return http.StatusConflict // 409

	// DOMAIN
	case apperr.DOMAIN_RULE_VIOLATION:
		return http.StatusUnprocessableEntity // 422

	// RATE
	case apperr.RATE_LIMIT_EXCEEDED:
		return http.StatusTooManyRequests // 429
	case apperr.UPLOAD_SIZE_EXCEEDED:
		return http.StatusRequestEntityTooLarge // 413

	// INFRA
	case apperr.INFRA_EXTERNAL_SERVICE_ERROR:
		return http.StatusBadGateway // 502
	case apperr.INFRA_TIMEOUT:
		return http.StatusGatewayTimeout // 504
	case apperr.INFRA_AUTHENTICATION_ERROR,
		apperr.INFRA_DATABASE_ERROR,
		apperr.INFRA_STORAGE_ERROR:
		return http.StatusInternalServerError // 500

	// INTERNAL
	case apperr.INTERNAL_ERROR:
		fallthrough
	default:
		return http.StatusInternalServerError // 500
	}
}
