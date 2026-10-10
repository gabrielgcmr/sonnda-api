// internal/config/http.go
package config

const envPort = "APP_PORT"

type HTTPConfig struct {
	Port string
}

func loadHTTPConfig() HTTPConfig {
	return HTTPConfig{
		Port: getEnvOrDefault(envPort, "8080"),
	}
}
