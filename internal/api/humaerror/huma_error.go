// internal/api/humaerror/huma_error.go
package humaerror

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

// Error exposes only Huma's public model while retaining the original error
// for errors.Is/errors.As and request-scoped diagnostics.
type Error struct {
	*huma.ErrorModel
	cause error
	code  apperr.ErrorKind
}

func (e *Error) Unwrap() error { return e.cause }

// From translates an application error without serializing its internal cause.
func From(err error) huma.StatusError {
	return from(err)
}

func from(err error) *Error {
	model := &huma.ErrorModel{
		Status: http.StatusInternalServerError,
		Title:  http.StatusText(http.StatusInternalServerError),
		Detail: "erro inesperado",
	}
	result := &Error{ErrorModel: model, cause: err, code: apperr.INTERNAL_ERROR}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr == nil {
		return result
	}

	result.code = appErr.Kind
	model.Status = StatusFromKind(appErr.Kind)
	model.Title = http.StatusText(model.Status)
	model.Detail = appErr.Message
	for _, violation := range appErr.Violations {
		model.Errors = append(model.Errors, &huma.ErrorDetail{
			Location: violation.Field,
			Message:  violation.Reason,
		})
	}

	return result
}

// Write writes an application error using Huma's configured error writer.
func Write(api huma.API, ctx huma.Context, err error) error {
	model := from(err)
	observe(ctx, model)
	details := make([]error, len(model.Errors))
	for i, detail := range model.Errors {
		details[i] = detail
	}
	return huma.WriteErr(api, ctx, model.Status, model.Detail, details...)
}

// StatusFromKind maps the stable application error code to an HTTP status.
func StatusFromKind(kind apperr.ErrorKind) int {
	switch kind {
	case apperr.AUTH_REQUIRED, apperr.AUTH_TOKEN_INVALID, apperr.AUTH_TOKEN_EXPIRED:
		return http.StatusUnauthorized
	case apperr.PROFILE_NOT_FOUND, apperr.ACCESS_DENIED, apperr.ACTION_NOT_ALLOWED,
		apperr.ONBOARDING_REQUIRED, apperr.ACCOUNT_DEACTIVATED:
		return http.StatusForbidden
	case apperr.VALIDATION_FAILED, apperr.REQUIRED_FIELD_MISSING, apperr.INVALID_FIELD_FORMAT, apperr.INVALID_ENUM_VALUE, apperr.INVALID_DATE:
		return http.StatusBadRequest
	case apperr.UNSUPPORTED_MEDIA_TYPE:
		return http.StatusUnsupportedMediaType
	case apperr.NOT_FOUND:
		return http.StatusNotFound
	case apperr.RESOURCE_CONFLICT, apperr.RESOURCE_ALREADY_EXISTS:
		return http.StatusConflict
	case apperr.DOMAIN_RULE_VIOLATION:
		return http.StatusUnprocessableEntity
	case apperr.RATE_LIMIT_EXCEEDED:
		return http.StatusTooManyRequests
	case apperr.UPLOAD_SIZE_EXCEEDED:
		return http.StatusRequestEntityTooLarge
	case apperr.INFRA_EXTERNAL_SERVICE_ERROR:
		return http.StatusBadGateway
	case apperr.INFRA_TIMEOUT:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
