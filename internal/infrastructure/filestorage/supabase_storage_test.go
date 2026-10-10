// internal/infrastructure/filestorage/supabase_storage_test.go
package filestorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

const (
	testSecretKey = "super-secret-supabase-test-key-12345"
	testBucket    = "exam-documents"
)

type customGenericReader struct {
	data   []byte
	offset int
}

func (r *customGenericReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func newTestStorageClient(t *testing.T, handler http.HandlerFunc) (*SupabaseStorageClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	client, err := NewSupabaseStorageClient(SupabaseClientConfig{
		ProjectURL: server.URL,
		SecretKey:  testSecretKey,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client, server
}

func TestNewSupabaseStorageClient_Validation(t *testing.T) {
	t.Run("missing ProjectURL", func(t *testing.T) {
		_, err := NewSupabaseStorageClient(SupabaseClientConfig{
			ProjectURL: "",
			SecretKey:  testSecretKey,
		})
		if err == nil {
			t.Fatal("expected error for empty ProjectURL")
		}
		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.REQUIRED_FIELD_MISSING {
			t.Fatalf("expected REQUIRED_FIELD_MISSING, got: %v", err)
		}
	})

	t.Run("invalid ProjectURL", func(t *testing.T) {
		_, err := NewSupabaseStorageClient(SupabaseClientConfig{
			ProjectURL: "ftp://invalid-scheme.com",
			SecretKey:  testSecretKey,
		})
		if err == nil {
			t.Fatal("expected error for invalid scheme")
		}
		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.INVALID_FIELD_FORMAT {
			t.Fatalf("expected INVALID_FIELD_FORMAT, got: %v", err)
		}
	})

	t.Run("missing SecretKey", func(t *testing.T) {
		_, err := NewSupabaseStorageClient(SupabaseClientConfig{
			ProjectURL: "https://example.supabase.co",
			SecretKey:  "",
		})
		if err == nil {
			t.Fatal("expected error for empty SecretKey")
		}
		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.REQUIRED_FIELD_MISSING {
			t.Fatalf("expected REQUIRED_FIELD_MISSING, got: %v", err)
		}
	})

	t.Run("normalize trailing slashes", func(t *testing.T) {
		client, err := NewSupabaseStorageClient(SupabaseClientConfig{
			ProjectURL: "https://example.supabase.co///",
			SecretKey:  testSecretKey,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.baseURL != "https://example.supabase.co" {
			t.Fatalf("expected baseURL https://example.supabase.co, got %s", client.baseURL)
		}
	})
}

func TestForBucket_Validation(t *testing.T) {
	client, err := NewSupabaseStorageClient(SupabaseClientConfig{
		ProjectURL: "https://example.supabase.co",
		SecretKey:  testSecretKey,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("empty bucket name", func(t *testing.T) {
		_, err := client.ForBucket(BucketConfig{BucketName: ""})
		if err == nil {
			t.Fatal("expected error for empty bucket name")
		}
		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.REQUIRED_FIELD_MISSING {
			t.Fatalf("expected REQUIRED_FIELD_MISSING, got: %v", err)
		}
	})

	t.Run("invalid bucket name characters", func(t *testing.T) {
		invalidNames := []string{"Bucket!", "exam documents", "-startdash", "enddash-", "a"}
		for _, name := range invalidNames {
			_, err := client.ForBucket(BucketConfig{BucketName: name})
			if err == nil {
				t.Fatalf("expected error for invalid bucket name %q", name)
			}
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Kind != apperr.INVALID_FIELD_FORMAT {
				t.Fatalf("expected INVALID_FIELD_FORMAT for %q, got: %v", name, err)
			}
		}
	})

	t.Run("default max file size is 5 MiB", func(t *testing.T) {
		storage, err := client.ForBucket(BucketConfig{BucketName: "exam-documents"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if storage.MaxFileSize() != DefaultMaxFileSize {
			t.Fatalf("expected default %d bytes (5 MiB), got: %d", DefaultMaxFileSize, storage.MaxFileSize())
		}
	})

	t.Run("custom max file size", func(t *testing.T) {
		storage, err := client.ForBucket(BucketConfig{
			BucketName:  "exam-documents",
			MaxFileSize: 2 * 1024 * 1024,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if storage.MaxFileSize() != 2*1024*1024 {
			t.Fatalf("expected 2 MiB, got: %d", storage.MaxFileSize())
		}
	})

	t.Run("canonical object URI", func(t *testing.T) {
		storage, err := client.ForBucket(BucketConfig{BucketName: "captures"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		uri, err := storage.ObjectURI("account/capture.pdf")
		if err != nil || uri != "supabase://captures/account/capture.pdf" {
			t.Fatalf("unexpected object URI: %q %v", uri, err)
		}
		if _, err := storage.ObjectURI("../capture.pdf"); err == nil {
			t.Fatal("expected invalid object path error")
		}
	})
}

func TestUpload(t *testing.T) {
	t.Run("success with headers, escaping and returned URI", func(t *testing.T) {
		var receivedAuth, receivedAPIKey, receivedContentType, receivedUpsert string
		var receivedPath, receivedMethod string
		var receivedBody []byte

		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedPath = r.URL.EscapedPath()
			receivedAuth = r.Header.Get("Authorization")
			receivedAPIKey = r.Header.Get("apikey")
			receivedContentType = r.Header.Get("Content-Type")
			receivedUpsert = r.Header.Get("x-upsert")
			receivedBody, _ = io.ReadAll(r.Body)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Key":"exam-documents/patients/1/doc file.pdf","Id":"id-123"}`))
		})
		defer server.Close()

		storage, err := client.ForBucket(BucketConfig{BucketName: testBucket})
		if err != nil {
			t.Fatalf("failed to create bucket storage: %v", err)
		}

		content := []byte("%PDF-1.4 sample content")
		objectName := "patients/1/doc file.pdf"
		uri, err := storage.Upload(context.Background(), bytes.NewReader(content), objectName, "application/pdf")
		if err != nil {
			t.Fatalf("upload failed: %v", err)
		}

		if receivedMethod != http.MethodPost {
			t.Errorf("expected POST, got %s", receivedMethod)
		}
		expectedPath := "/storage/v1/object/exam-documents/patients/1/doc%20file.pdf"
		if receivedPath != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, receivedPath)
		}
		if receivedAuth != "" {
			t.Errorf("expected no Authorization header for a Supabase secret key, got %s", receivedAuth)
		}
		if receivedAPIKey != testSecretKey {
			t.Errorf("expected apikey header, got %s", receivedAPIKey)
		}
		if receivedContentType != "application/pdf" {
			t.Errorf("expected application/pdf, got %s", receivedContentType)
		}
		if receivedUpsert != "false" {
			t.Errorf("expected x-upsert false, got %s", receivedUpsert)
		}
		if !bytes.Equal(receivedBody, content) {
			t.Errorf("expected body %q, got %q", string(content), string(receivedBody))
		}

		expectedURI := "supabase://exam-documents/patients/1/doc file.pdf"
		if uri != expectedURI {
			t.Errorf("expected URI %s, got %s", expectedURI, uri)
		}
	})

	t.Run("conflict error mapped to RESOURCE_ALREADY_EXISTS (HTTP 409)", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"statusCode":"409","error":"Duplicate","message":"The resource already exists"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.Upload(context.Background(), strings.NewReader("data"), "test.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected conflict error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.RESOURCE_ALREADY_EXISTS {
			t.Fatalf("expected RESOURCE_ALREADY_EXISTS, got: %v", err)
		}
	})

	t.Run("conflict error mapped to RESOURCE_ALREADY_EXISTS (HTTP 400 with KeyAlreadyExists)", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"statusCode":"409","error":"Duplicate","message":"The resource already exists","code":"KeyAlreadyExists"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.Upload(context.Background(), strings.NewReader("data"), "test.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected conflict error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.RESOURCE_ALREADY_EXISTS {
			t.Fatalf("expected RESOURCE_ALREADY_EXISTS, got: %v", err)
		}
	})

	t.Run("client-side max file size exceeded with sized reader", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{
			BucketName:  testBucket,
			MaxFileSize: 100, // 100 bytes limit
		})

		largeContent := bytes.Repeat([]byte("A"), 150)
		_, err := storage.Upload(context.Background(), bytes.NewReader(largeContent), "large.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected size exceeded error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.UPLOAD_SIZE_EXCEEDED {
			t.Fatalf("expected UPLOAD_SIZE_EXCEEDED, got: %v", err)
		}
	})

	t.Run("client-side max file size exceeded with generic stream reader", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{
			BucketName:  testBucket,
			MaxFileSize: 100,
		})

		// unbuffered custom reader without Len or Size methods
		genericReader := &customGenericReader{data: bytes.Repeat([]byte("B"), 150)}
		_, err := storage.Upload(context.Background(), genericReader, "stream.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected size exceeded error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.UPLOAD_SIZE_EXCEEDED {
			t.Fatalf("expected UPLOAD_SIZE_EXCEEDED, got: %v", err)
		}
	})

	t.Run("server-side 413 Payload Too Large mapped to UPLOAD_SIZE_EXCEEDED", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`{"statusCode":"413","error":"Payload Too Large","message":"The object exceeded the maximum allowed size"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.Upload(context.Background(), strings.NewReader("data"), "oversized.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected size exceeded error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.UPLOAD_SIZE_EXCEEDED {
			t.Fatalf("expected UPLOAD_SIZE_EXCEEDED, got: %v", err)
		}
	})

	t.Run("server-side 401 mapped to INFRA_AUTHENTICATION_ERROR", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"statusCode":"401","error":"Unauthorized","message":"Invalid token"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.Upload(context.Background(), strings.NewReader("data"), "test.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected auth error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.INFRA_AUTHENTICATION_ERROR {
			t.Fatalf("expected INFRA_AUTHENTICATION_ERROR, got: %v", err)
		}
	})

	t.Run("server-side 403 mapped to INFRA_AUTHENTICATION_ERROR", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"statusCode":"403","error":"AccessDenied","message":"Forbidden"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.Upload(context.Background(), strings.NewReader("data"), "test.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected access denied error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.INFRA_AUTHENTICATION_ERROR {
			t.Fatalf("expected INFRA_AUTHENTICATION_ERROR, got: %v", err)
		}
	})

	t.Run("server-side 500 mapped to INFRA_STORAGE_ERROR", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"statusCode":"500","error":"Internal Server Error","message":"Database error"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.Upload(context.Background(), strings.NewReader("data"), "test.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected storage error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.INFRA_STORAGE_ERROR {
			t.Fatalf("expected INFRA_STORAGE_ERROR, got: %v", err)
		}
	})

	t.Run("invalid object names", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		invalidNames := []string{
			"",
			"/leading/slash.pdf",
			"trailing/slash/",
			"path/../traversal.pdf",
			"path/./current.pdf",
			"path//doubleslash.pdf",
		}

		for _, name := range invalidNames {
			_, err := storage.Upload(context.Background(), strings.NewReader("data"), name, "application/pdf")
			if err == nil {
				t.Fatalf("expected error for object name %q", name)
			}
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Kind != apperr.VALIDATION_FAILED {
				t.Fatalf("expected VALIDATION_FAILED for %q, got: %v", name, err)
			}
		}
	})

	t.Run("context cancellation mapped to INFRA_TIMEOUT", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := storage.Upload(ctx, strings.NewReader("data"), "test.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected timeout/canceled error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.INFRA_TIMEOUT {
			t.Fatalf("expected INFRA_TIMEOUT, got: %v", err)
		}
	})
}

func TestOpen(t *testing.T) {
	t.Run("success streaming authenticated object", func(t *testing.T) {
		var receivedAuth, receivedAPIKey, receivedMethod, receivedPath string

		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedPath = r.URL.EscapedPath()
			receivedAuth = r.Header.Get("Authorization")
			receivedAPIKey = r.Header.Get("apikey")

			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("%PDF-stream-data"))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		uri := "supabase://exam-documents/patients/p-1/doc.pdf"

		reader, err := storage.Open(context.Background(), uri)
		if err != nil {
			t.Fatalf("open failed: %v", err)
		}
		defer reader.Close()

		if receivedMethod != http.MethodGet {
			t.Errorf("expected GET, got %s", receivedMethod)
		}
		expectedPath := "/storage/v1/object/authenticated/exam-documents/patients/p-1/doc.pdf"
		if receivedPath != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, receivedPath)
		}
		if receivedAuth != "" {
			t.Errorf("expected no Authorization header for a Supabase secret key, got %s", receivedAuth)
		}
		if receivedAPIKey != testSecretKey {
			t.Errorf("expected apikey header, got %s", receivedAPIKey)
		}

		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read stream failed: %v", err)
		}
		if string(body) != "%PDF-stream-data" {
			t.Fatalf("expected stream content %%PDF-stream-data, got %s", string(body))
		}
	})

	t.Run("not found mapped to NOT_FOUND (HTTP 404)", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"statusCode":"404","error":"not_found","message":"Object not found"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.Open(context.Background(), "supabase://exam-documents/missing.pdf")
		if err == nil {
			t.Fatal("expected not found error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.NOT_FOUND {
			t.Fatalf("expected NOT_FOUND, got: %v", err)
		}
	})

	t.Run("not found mapped to NOT_FOUND (HTTP 400 NoSuchKey)", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"statusCode":"404","error":"not_found","message":"Object not found","code":"NoSuchKey"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.Open(context.Background(), "supabase://exam-documents/missing.pdf")
		if err == nil {
			t.Fatal("expected not found error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.NOT_FOUND {
			t.Fatalf("expected NOT_FOUND, got: %v", err)
		}
	})

	t.Run("invalid URI scheme, bucket mismatch, traversal", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})

		testCases := []struct {
			uri string
			msg string
		}{
			{"gs://exam-documents/doc.pdf", "wrong scheme"},
			{"supabase://captures/doc.pdf", "bucket mismatch"},
			{"supabase://exam-documents/../secret.pdf", "traversal"},
			{"supabase://exam-documents/", "empty path"},
		}

		for _, tc := range testCases {
			_, err := storage.Open(context.Background(), tc.uri)
			if err == nil {
				t.Fatalf("expected error for %s (%s)", tc.uri, tc.msg)
			}
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) || appErr.Kind != apperr.VALIDATION_FAILED {
				t.Fatalf("expected VALIDATION_FAILED for %s, got: %v", tc.uri, err)
			}
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("idempotent delete existing file", func(t *testing.T) {
		var receivedMethod, receivedPath string
		var receivedPayload struct {
			Prefixes []string `json:"prefixes"`
		}

		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&receivedPayload)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"name":"patients/1/doc.pdf","id":"abc"}]`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		err := storage.Delete(context.Background(), "supabase://exam-documents/patients/1/doc.pdf")
		if err != nil {
			t.Fatalf("delete failed: %v", err)
		}

		if receivedMethod != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", receivedMethod)
		}
		if receivedPath != "/storage/v1/object/exam-documents" {
			t.Errorf("expected /storage/v1/object/exam-documents, got %s", receivedPath)
		}
		if len(receivedPayload.Prefixes) != 1 || receivedPayload.Prefixes[0] != "patients/1/doc.pdf" {
			t.Errorf("expected prefixes [patients/1/doc.pdf], got %v", receivedPayload.Prefixes)
		}
	})

	t.Run("idempotent delete repeated on non-existing file returns 200 with empty array", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		err := storage.Delete(context.Background(), "supabase://exam-documents/already-deleted.pdf")
		if err != nil {
			t.Fatalf("delete should be idempotent and succeed: %v", err)
		}
	})
}

func TestGetSignedURL(t *testing.T) {
	t.Run("resolves relative signedURL starting with /object/sign/", func(t *testing.T) {
		var receivedMethod, receivedPath string
		var receivedPayload struct {
			ExpiresIn int `json:"expiresIn"`
		}

		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&receivedPayload)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"signedURL":"/object/sign/exam-documents/patients/1/doc.pdf?token=jwt-token-123"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		signedURL, err := storage.GetSignedURL(context.Background(), "supabase://exam-documents/patients/1/doc.pdf", 15*time.Minute)
		if err != nil {
			t.Fatalf("get signed url failed: %v", err)
		}

		if receivedMethod != http.MethodPost {
			t.Errorf("expected POST, got %s", receivedMethod)
		}
		if receivedPath != "/storage/v1/object/sign/exam-documents/patients/1/doc.pdf" {
			t.Errorf("expected /storage/v1/object/sign/exam-documents/patients/1/doc.pdf, got %s", receivedPath)
		}
		if receivedPayload.ExpiresIn != 900 {
			t.Errorf("expected expiresIn 900 seconds, got %d", receivedPayload.ExpiresIn)
		}

		expectedURL := server.URL + "/storage/v1/object/sign/exam-documents/patients/1/doc.pdf?token=jwt-token-123"
		if signedURL != expectedURL {
			t.Fatalf("expected signedURL %s, got %s", expectedURL, signedURL)
		}
	})

	t.Run("resolves relative signedURL starting with /storage/v1/object/sign/", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"signedURL":"/storage/v1/object/sign/exam-documents/patients/1/doc.pdf?token=jwt-token-123"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		signedURL, err := storage.GetSignedURL(context.Background(), "supabase://exam-documents/patients/1/doc.pdf", 5*time.Minute)
		if err != nil {
			t.Fatalf("get signed url failed: %v", err)
		}

		expectedURL := server.URL + "/storage/v1/object/sign/exam-documents/patients/1/doc.pdf?token=jwt-token-123"
		if signedURL != expectedURL {
			t.Fatalf("expected signedURL %s, got %s", expectedURL, signedURL)
		}
	})

	t.Run("invalid expiresIn <= 0", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.GetSignedURL(context.Background(), "supabase://exam-documents/doc.pdf", 0)
		if err == nil {
			t.Fatal("expected error for 0 duration")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.VALIDATION_FAILED {
			t.Fatalf("expected VALIDATION_FAILED, got: %v", err)
		}
	})

	t.Run("sign non-existing object mapped to NOT_FOUND", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"statusCode":"404","error":"not_found","message":"Object not found","code":"NoSuchKey"}`))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.GetSignedURL(context.Background(), "supabase://exam-documents/missing.pdf", 5*time.Minute)
		if err == nil {
			t.Fatal("expected not found error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.NOT_FOUND {
			t.Fatalf("expected NOT_FOUND, got: %v", err)
		}
	})

	t.Run("oversized response body mapped to INFRA_STORAGE_ERROR", func(t *testing.T) {
		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte("x"), maxResponseBodySize+1))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})
		_, err := storage.GetSignedURL(context.Background(), "supabase://exam-documents/doc.pdf", 5*time.Minute)
		if err == nil {
			t.Fatal("expected oversized response error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) || appErr.Kind != apperr.INFRA_STORAGE_ERROR {
			t.Fatalf("expected INFRA_STORAGE_ERROR, got: %v", err)
		}
	})
}

func TestSecurity_Sanitization(t *testing.T) {
	t.Run("never leak secret key or signed url token in AppError cause or message", func(t *testing.T) {
		secretLeak := testSecretKey
		tokenLeak := "super-confidential-token-999"

		client, server := newTestStorageClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			// Simula um erro do servidor que ecoa a chave e um token de URL
			errorBody := fmt.Sprintf(`{"message":"server failed with key=%s and url=/object/sign/doc.pdf?token=%s"}`, secretLeak, tokenLeak)
			_, _ = w.Write([]byte(errorBody))
		})
		defer server.Close()

		storage, _ := client.ForBucket(BucketConfig{BucketName: testBucket})

		_, err := storage.Upload(context.Background(), strings.NewReader("content"), "doc.pdf", "application/pdf")
		if err == nil {
			t.Fatal("expected error")
		}

		var appErr *apperr.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got: %v", err)
		}

		// Valida que a secret key não está em lugar nenhum
		if strings.Contains(appErr.Message, secretLeak) {
			t.Errorf("secret key leaked in AppError.Message: %s", appErr.Message)
		}
		if appErr.Cause != nil && strings.Contains(appErr.Cause.Error(), secretLeak) {
			t.Errorf("secret key leaked in AppError.Cause: %s", appErr.Cause.Error())
		}
		if strings.Contains(appErr.Error(), secretLeak) {
			t.Errorf("secret key leaked in appErr.Error(): %s", appErr.Error())
		}

		// Valida que o token não está em lugar nenhum
		if strings.Contains(appErr.Message, tokenLeak) {
			t.Errorf("token leaked in AppError.Message: %s", appErr.Message)
		}
		if appErr.Cause != nil && strings.Contains(appErr.Cause.Error(), tokenLeak) {
			t.Errorf("token leaked in AppError.Cause: %s", appErr.Cause.Error())
		}
		if strings.Contains(appErr.Error(), tokenLeak) {
			t.Errorf("token leaked in appErr.Error(): %s", appErr.Error())
		}
	})
}
