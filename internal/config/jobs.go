// internal/config/jobs.go
package config

const envCaptureCleanupToken = "CAPTURE_CLEANUP_TOKEN"

type JobsConfig struct {
	CaptureCleanupToken string
}

func loadJobsConfig() JobsConfig {
	return JobsConfig{CaptureCleanupToken: getEnv(envCaptureCleanupToken)}
}
