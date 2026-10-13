// internal/config/storage.go
package config

const (
	envSupabaseSecretKey           = "SUPABASE_SECRET_KEY"
	envSupabaseExamDocumentsBucket = "SUPABASE_EXAM_DOCUMENTS_BUCKET"
	envSupabaseCapturesBucket      = "SUPABASE_CAPTURES_BUCKET"
	envSupabaseTemporaryBucket     = "SUPABASE_TEMPORARY_EXTRACTIONS_BUCKET"
)

type StorageConfig struct {
	SupabaseSecretKey           string
	SupabaseExamDocumentsBucket string
	SupabaseCapturesBucket      string
	SupabaseTemporaryBucket     string
}

func loadStorageConfig() StorageConfig {
	temporaryBucket := getEnv(envSupabaseTemporaryBucket)
	if temporaryBucket == "" {
		temporaryBucket = "temporary-extractions"
	}
	return StorageConfig{
		SupabaseSecretKey:           getEnv(envSupabaseSecretKey),
		SupabaseExamDocumentsBucket: getEnv(envSupabaseExamDocumentsBucket),
		SupabaseCapturesBucket:      getEnv(envSupabaseCapturesBucket),
		SupabaseTemporaryBucket:     temporaryBucket,
	}
}
