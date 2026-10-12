// internal/config/gemini.go
package config

import (
	"strconv"
	"strings"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type GeminiConfig struct {
	APIKey          string `json:"-"`
	Model           string
	Timeout         time.Duration
	MaxInputBytes   int
	MaxMediaBytes   int
	MaxOutputTokens int32
}

func LoadGemini() (GeminiConfig, error) {
	return loadGeminiConfig()
}

func loadGeminiConfig() (GeminiConfig, error) {
	timeout, err := time.ParseDuration(getEnvOrDefault("GEMINI_TIMEOUT", "30s"))
	if err != nil {
		return GeminiConfig{}, apperr.Validation("invalid configuration", apperr.Violation{Field: "GEMINI_TIMEOUT", Reason: "invalid_duration"})
	}
	inputLimit, err := strconv.Atoi(getEnvOrDefault("GEMINI_MAX_INPUT_BYTES", "131072"))
	if err != nil {
		return GeminiConfig{}, apperr.Validation("invalid configuration", apperr.Violation{Field: "GEMINI_MAX_INPUT_BYTES", Reason: "invalid_integer"})
	}
	mediaLimit, err := strconv.Atoi(getEnvOrDefault("GEMINI_MAX_MEDIA_BYTES", "10485760"))
	if err != nil {
		return GeminiConfig{}, apperr.Validation("invalid configuration", apperr.Violation{Field: "GEMINI_MAX_MEDIA_BYTES", Reason: "invalid_integer"})
	}
	outputLimit, err := strconv.ParseInt(getEnvOrDefault("GEMINI_MAX_OUTPUT_TOKENS", "8192"), 10, 32)
	if err != nil {
		return GeminiConfig{}, apperr.Validation("invalid configuration", apperr.Violation{Field: "GEMINI_MAX_OUTPUT_TOKENS", Reason: "invalid_integer"})
	}
	cfg := GeminiConfig{
		APIKey:          getEnv("GEMINI_API_KEY"),
		Model:           getEnvOrDefault("GEMINI_MODEL", "gemini-3.5-flash-lite"),
		Timeout:         timeout,
		MaxInputBytes:   inputLimit,
		MaxMediaBytes:   mediaLimit,
		MaxOutputTokens: int32(outputLimit),
	}
	return cfg, cfg.Validate()
}

// Validate confere limites; a chave so e exigida ao criar o cliente real.
func (cfg GeminiConfig) Validate() error {
	var violations []apperr.Violation
	if strings.TrimSpace(cfg.Model) == "" {
		violations = append(violations, apperr.Violation{Field: "GEMINI_MODEL", Reason: "required"})
	}
	if cfg.Timeout <= 0 {
		violations = append(violations, apperr.Violation{Field: "GEMINI_TIMEOUT", Reason: "must_be_positive"})
	}
	if cfg.MaxInputBytes <= 0 {
		violations = append(violations, apperr.Violation{Field: "GEMINI_MAX_INPUT_BYTES", Reason: "must_be_positive"})
	}
	if cfg.MaxMediaBytes <= 0 {
		violations = append(violations, apperr.Violation{Field: "GEMINI_MAX_MEDIA_BYTES", Reason: "must_be_positive"})
	}
	if cfg.MaxOutputTokens <= 0 {
		violations = append(violations, apperr.Violation{Field: "GEMINI_MAX_OUTPUT_TOKENS", Reason: "must_be_positive"})
	}
	if len(violations) > 0 {
		return apperr.Validation("invalid configuration", violations...)
	}
	return nil
}
