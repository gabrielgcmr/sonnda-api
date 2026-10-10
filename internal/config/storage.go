// internal/config/storage.go
package config

const (
	envSupabaseSecretKey           = "SUPABASE_SECRET_KEY"
	envSupabaseExamDocumentsBucket = "SUPABASE_EXAM_DOCUMENTS_BUCKET"
	envSupabaseCapturesBucket      = "SUPABASE_CAPTURES_BUCKET"
)

type StorageConfig struct {
	SupabaseSecretKey           string
	SupabaseExamDocumentsBucket string
	SupabaseCapturesBucket      string
}

func loadStorageConfig() StorageConfig {
	return StorageConfig{
		SupabaseSecretKey:           getEnv(envSupabaseSecretKey),
		SupabaseExamDocumentsBucket: getEnv(envSupabaseExamDocumentsBucket),
		SupabaseCapturesBucket:      getEnv(envSupabaseCapturesBucket),
	}
}
