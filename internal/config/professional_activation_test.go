// internal/config/professional_activation_test.go
package config

import (
	"testing"
	"time"
)

func TestProfessionalActivationConfigDefaultsAndValidation(t *testing.T) {
	t.Setenv(envProfessionalActivationPasswordHash, "hash-value")
	t.Setenv(envProfessionalActivationMaxAttempts, "")
	t.Setenv(envProfessionalActivationWindow, "")
	config, err := loadProfessionalActivationConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.PasswordHash != "hash-value" || config.MaxAttempts != 5 || config.Window != 15*time.Minute {
		t.Fatalf("unexpected config: %+v", config)
	}

	t.Setenv(envProfessionalActivationMaxAttempts, "0")
	if _, err := loadProfessionalActivationConfig(); err == nil {
		t.Fatal("zero max attempts must be rejected")
	}
	t.Setenv(envProfessionalActivationMaxAttempts, "5")
	t.Setenv(envProfessionalActivationWindow, "invalid")
	if _, err := loadProfessionalActivationConfig(); err == nil {
		t.Fatal("invalid window must be rejected")
	}
}
