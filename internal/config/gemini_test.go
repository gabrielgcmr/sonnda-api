// internal/config/gemini_test.go
package config

import (
	"errors"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

func clearGeminiEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GEMINI_API_KEY", "GEMINI_MODEL", "GEMINI_TIMEOUT", "GEMINI_MAX_INPUT_BYTES", "GEMINI_MAX_MEDIA_BYTES", "GEMINI_MAX_OUTPUT_TOKENS"} {
		t.Setenv(key, "")
	}
}

func TestGeminiConfigDefaultsWithoutCredentials(t *testing.T) {
	clearGeminiEnv(t)
	cfg, err := loadGeminiConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "" || cfg.Model != "gemini-3.5-flash-lite" || cfg.Timeout != 30*time.Second || cfg.MaxInputBytes != 131072 || cfg.MaxMediaBytes != 10*1024*1024 || cfg.MaxOutputTokens != 8192 {
		t.Fatal("unexpected Gemini defaults")
	}
}

func TestGeminiConfigOverrides(t *testing.T) {
	clearGeminiEnv(t)
	t.Setenv("GEMINI_API_KEY", " test-key ")
	t.Setenv("GEMINI_MODEL", " test-model ")
	t.Setenv("GEMINI_TIMEOUT", "5s")
	t.Setenv("GEMINI_MAX_INPUT_BYTES", "1024")
	t.Setenv("GEMINI_MAX_MEDIA_BYTES", "2048")
	t.Setenv("GEMINI_MAX_OUTPUT_TOKENS", "256")
	cfg, err := loadGeminiConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "test-key" || cfg.Model != "test-model" || cfg.Timeout != 5*time.Second || cfg.MaxInputBytes != 1024 || cfg.MaxMediaBytes != 2048 || cfg.MaxOutputTokens != 256 {
		t.Fatal("Gemini configuration overrides were not applied")
	}
}

func TestGeminiConfigRejectsInvalidLimits(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"GEMINI_TIMEOUT", "invalid"},
		{"GEMINI_TIMEOUT", "0s"},
		{"GEMINI_TIMEOUT", "-1s"},
		{"GEMINI_MAX_INPUT_BYTES", "text"},
		{"GEMINI_MAX_INPUT_BYTES", "0"},
		{"GEMINI_MAX_INPUT_BYTES", "-1"},
		{"GEMINI_MAX_MEDIA_BYTES", "text"},
		{"GEMINI_MAX_MEDIA_BYTES", "0"},
		{"GEMINI_MAX_MEDIA_BYTES", "-1"},
		{"GEMINI_MAX_OUTPUT_TOKENS", "1.5"},
		{"GEMINI_MAX_OUTPUT_TOKENS", "-1"},
		{"GEMINI_MAX_OUTPUT_TOKENS", "0"},
		{"GEMINI_MAX_OUTPUT_TOKENS", "2147483648"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			clearGeminiEnv(t)
			t.Setenv(tc.key, tc.value)
			_, err := loadGeminiConfig()
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || len(appErr.Violations) != 1 || appErr.Violations[0].Field != tc.key {
				t.Fatalf("expected configuration violation for %s", tc.key)
			}
		})
	}
}
