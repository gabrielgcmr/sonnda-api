// internal/config/load.go
package config

import (
	"github.com/joho/godotenv"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

func Load() (*Config, error) {
	// Carrega variáveis do arquivo .env
	_ = godotenv.Load()

	appCfg := loadAppConfig()
	ocrCfg, err := loadOCRConfig()
	if err != nil {
		return nil, err
	}
	geminiCfg, err := loadGeminiConfig()
	if err != nil {
		return nil, err
	}
	professionalActivationCfg, err := loadProfessionalActivationConfig()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		App:                    appCfg,
		HTTP:                   loadHTTPConfig(),
		Database:               loadDatabaseConfig(),
		Auth:                   loadAuthConfig(),
		Storage:                loadStorageConfig(),
		CORS:                   loadCORSConfig(appCfg.Env),
		Gemini:                 geminiCfg,
		OCR:                    ocrCfg,
		ProfessionalActivation: professionalActivationCfg,
		Jobs:                   loadJobsConfig(),
	}

	var violations []apperr.Violation

	appendRequired(&violations, envDatabaseURL, cfg.Database.URL)
	appendRequired(&violations, envSupabaseProjectURL, cfg.Auth.SupabaseProjectURL)
	appendRequired(&violations, envGCPProjectID, cfg.Storage.GCPProjectID)
	appendRequired(&violations, envGCSBucket, cfg.Storage.GCSBucket)
	appendRequired(&violations, envSupabaseSecretKey, cfg.Storage.SupabaseSecretKey)
	appendRequired(&violations, envSupabaseCapturesBucket, cfg.Storage.SupabaseCapturesBucket)
	appendRequired(&violations, envCaptureCleanupToken, cfg.Jobs.CaptureCleanupToken)
	// Exigir pelo menos uma forma de credenciais do Google Cloud
	if cfg.Storage.GoogleApplicationCredentials == "" && cfg.Storage.GoogleApplicationCredentialsJSON == "" {
		violations = append(violations, apperr.Violation{
			Field:  "GOOGLE_CREDENTIALS",
			Reason: "either GOOGLE_APPLICATION_CREDENTIALS or GOOGLE_APPLICATION_CREDENTIALS_JSON is required",
		})
	}
	validateEnum(&violations, envAppEnv, cfg.App.Env, allowedEnvs)
	validateEnum(&violations, envLogLevel, cfg.App.LogLevel, allowedLogLevels)
	validateEnum(&violations, envLogFormat, cfg.App.LogFormat, allowedLogFormats)

	if len(violations) > 0 {
		return nil, apperr.Validation("invalid configuration", violations...)
	}

	if cfg.App.Env == "prod" && cfg.App.LogFormat == "text" {
		cfg.App.LogFormat = "json"
	}

	return cfg, nil
}
