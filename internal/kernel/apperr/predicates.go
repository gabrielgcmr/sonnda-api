// internal/kernel/apperr/predicates.go
package apperr

// HasCode retorna true se `err` (ou algum erro na sua cadeia) for um *AppError
// com um dos códigos informados.
func HasCode(err error, codes ...ErrorKind) bool {
	if err == nil || len(codes) == 0 {
		return false
	}

	code := ErrorCodeOf(err)
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

func IsNotFound(err error) bool {
	return HasCode(err, NOT_FOUND)
}

// IsValidation cobre erros de validação/entrada (4xx) retornados como contrato.
func IsValidation(err error) bool {
	return HasCode(err,
		VALIDATION_FAILED,
		REQUIRED_FIELD_MISSING,
		INVALID_FIELD_FORMAT,
		INVALID_ENUM_VALUE,
		INVALID_DATE,
	)
}

func IsUnauthorized(err error) bool {
	return HasCode(err, AUTH_REQUIRED, AUTH_TOKEN_INVALID, AUTH_TOKEN_EXPIRED)
}

func IsForbidden(err error) bool {
	return HasCode(err, ACCESS_DENIED, ACTION_NOT_ALLOWED, PROFILE_NOT_FOUND, ONBOARDING_REQUIRED, ACCOUNT_DEACTIVATED)
}

func IsConflict(err error) bool {
	return HasCode(err, RESOURCE_CONFLICT, RESOURCE_ALREADY_EXISTS)
}

func IsDomainRuleViolation(err error) bool {
	return HasCode(err, DOMAIN_RULE_VIOLATION)
}

func IsRateLimited(err error) bool {
	return HasCode(err, RATE_LIMIT_EXCEEDED)
}

func IsPayloadTooLarge(err error) bool {
	return HasCode(err, UPLOAD_SIZE_EXCEEDED)
}

func IsInfra(err error) bool {
	return HasCode(err,
		INFRA_AUTHENTICATION_ERROR,
		INFRA_DATABASE_ERROR,
		INFRA_STORAGE_ERROR,
		INFRA_EXTERNAL_SERVICE_ERROR,
		INFRA_TIMEOUT,
	)
}

func IsInternal(err error) bool {
	return HasCode(err, INTERNAL_ERROR)
}
