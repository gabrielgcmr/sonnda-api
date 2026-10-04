// internal/config/professional_activation.go
package config

import (
	"strconv"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

const (
	envProfessionalActivationPasswordHash = "PROFESSIONAL_ACTIVATION_PASSWORD_HASH"
	envProfessionalActivationMaxAttempts  = "PROFESSIONAL_ACTIVATION_MAX_ATTEMPTS"
	envProfessionalActivationWindow       = "PROFESSIONAL_ACTIVATION_WINDOW"
)

type ProfessionalActivationConfig struct {
	PasswordHash string
	MaxAttempts  int
	Window       time.Duration
}

func loadProfessionalActivationConfig() (ProfessionalActivationConfig, error) {
	maxAttempts, err := strconv.Atoi(getEnvOrDefault(envProfessionalActivationMaxAttempts, "5"))
	if err != nil || maxAttempts < 1 {
		return ProfessionalActivationConfig{}, apperr.Validation("invalid configuration", apperr.Violation{
			Field: envProfessionalActivationMaxAttempts, Reason: "must be a positive integer",
		})
	}
	window, err := time.ParseDuration(getEnvOrDefault(envProfessionalActivationWindow, "15m"))
	if err != nil || window <= 0 {
		return ProfessionalActivationConfig{}, apperr.Validation("invalid configuration", apperr.Violation{
			Field: envProfessionalActivationWindow, Reason: "must be a positive duration",
		})
	}
	return ProfessionalActivationConfig{
		PasswordHash: getEnv(envProfessionalActivationPasswordHash),
		MaxAttempts:  maxAttempts,
		Window:       window,
	}, nil
}
