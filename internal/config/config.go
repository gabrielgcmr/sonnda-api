// internal/config/config.go
package config

type Config struct {
	App                    AppConfig
	HTTP                   HTTPConfig
	Database               DatabaseConfig
	Auth                   AuthConfig
	Storage                StorageConfig
	CORS                   CORSConfig
	Gemini                 GeminiConfig
	OCR                    OCRConfig
	ProfessionalActivation ProfessionalActivationConfig
}
