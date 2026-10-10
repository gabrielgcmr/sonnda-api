// internal/config/storage.go
package config

const (
	envGoogleApplicationCredentials     = "GOOGLE_APPLICATION_CREDENTIALS"
	envGoogleApplicationCredentialsJSON = "GOOGLE_APPLICATION_CREDENTIALS_JSON"
	envGCPProjectID                     = "GCP_PROJECT_ID"
	envGCPProjectNumber                 = "GCP_PROJECT_NUMBER"
	envGCSBucket                        = "GCS_BUCKET"
	envGCPLocation                      = "GCP_LOCATION"
	envGCPExtractLabsProcessorID        = "GCP_EXTRACT_LABS_PROCESSOR_ID"
	envSupabaseSecretKey                = "SUPABASE_SECRET_KEY"
	envSupabaseCapturesBucket           = "SUPABASE_CAPTURES_BUCKET"
)

type StorageConfig struct {
	GoogleApplicationCredentials     string
	GoogleApplicationCredentialsJSON string
	GCPProjectID                     string
	GCPProjectNumber                 string
	GCSBucket                        string
	GCPLocation                      string
	GCPExtractLabsProcessorID        string
	SupabaseSecretKey                string
	SupabaseCapturesBucket           string
}

func loadStorageConfig() StorageConfig {
	return StorageConfig{
		GoogleApplicationCredentials:     getEnv(envGoogleApplicationCredentials),
		GoogleApplicationCredentialsJSON: getEnv(envGoogleApplicationCredentialsJSON),
		GCPProjectID:                     getEnv(envGCPProjectID),
		GCPProjectNumber:                 getEnv(envGCPProjectNumber),
		GCSBucket:                        getEnv(envGCSBucket),
		GCPLocation:                      getEnv(envGCPLocation),
		GCPExtractLabsProcessorID:        getEnv(envGCPExtractLabsProcessorID),
		SupabaseSecretKey:                getEnv(envSupabaseSecretKey),
		SupabaseCapturesBucket:           getEnv(envSupabaseCapturesBucket),
	}
}
