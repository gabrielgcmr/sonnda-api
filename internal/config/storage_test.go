// internal/config/storage_test.go
package config

import "testing"

func TestLoadStorageConfig(t *testing.T) {
	t.Setenv(envSupabaseSecretKey, "sb_secret_test")
	t.Setenv(envSupabaseExamDocumentsBucket, "exam-documents")
	t.Setenv(envSupabaseCapturesBucket, "captures")

	cfg := loadStorageConfig()

	if cfg.SupabaseSecretKey != "sb_secret_test" {
		t.Fatalf("SupabaseSecretKey = %q", cfg.SupabaseSecretKey)
	}
	if cfg.SupabaseExamDocumentsBucket != "exam-documents" {
		t.Fatalf("SupabaseExamDocumentsBucket = %q", cfg.SupabaseExamDocumentsBucket)
	}
	if cfg.SupabaseCapturesBucket != "captures" {
		t.Fatalf("SupabaseCapturesBucket = %q", cfg.SupabaseCapturesBucket)
	}
}
