// internal/config/cors.go
package config

import (
	"strings"
	"time"
)

const (
	envCORSOrigins     = "CORS_ORIGINS"
	envCORSCredentials = "CORS_CREDENTIALS"
)

type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           time.Duration
}

func loadCORSConfig(appEnv string) CORSConfig {
	// Defaults variam por ambiente
	var defaultOrigins string
	maxAgeHours := 12

	switch appEnv {
	case "prod":
		// Produção: apenas o domínio do frontend em produção
		defaultOrigins = "https://app.sonnda.com.br"
		maxAgeHours = 24
	case "staging":
		// Staging
		defaultOrigins = "https://app-staging.sonnda.com.br"
	default:
		// Desenvolvimento: aceita localhost com porta dinâmica (Flutter Web/Chrome)
		defaultOrigins = "http://localhost:*,http://127.0.0.1:*,http://localhost:5173,http://localhost:3000"
	}

	originsStr := getEnvOrDefault(envCORSOrigins, defaultOrigins)
	origins := parseCommaSeparatedList(originsStr)

	credentialsStr := getEnvOrDefault(envCORSCredentials, "true")
	allowCredentials := credentialsStr != "false" && credentialsStr != "0"

	return CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Request-ID", "X-Capture-Token"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: allowCredentials,
		MaxAge:           time.Duration(maxAgeHours) * time.Hour,
	}
}

// parseCommaSeparatedList converte "a,b,c" em []string{"a", "b", "c"}
// com trim de espaços em branco
func parseCommaSeparatedList(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
