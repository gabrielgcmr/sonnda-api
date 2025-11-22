package exam

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"time"

	"cloud.google.com/go/storage"
)

type StorageClient interface {
	UploadFile(ctx context.Context, file *multipart.FileHeader, objectName string) (string, error)
	DeleteFile(ctx context.Context, objectName string) error
	SignedURL(ctx context.Context, objectName string, expires time.Duration) (string, error)
}

type GCSStorage struct {
	client     *storage.Client
	bucketName string
}

func NewGCSStorage(client *storage.Client, bucket string) *GCSStorage {
	return &GCSStorage{
		client:     client,
		bucketName: bucket,
	}
}

func (s *GCSStorage) UploadFile(ctx context.Context, file *multipart.FileHeader, objectName string) (string, error) {
	f, err := file.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := s.client.Bucket(s.bucketName).Object(objectName).NewWriter(ctx)

	if _, err := io.Copy(w, f); err != nil {
		return "", fmt.Errorf("failed to copy file content: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	// URL pública assinada (24h)
	url := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.bucketName, objectName)
	return url, nil
}

func (s *GCSStorage) DeleteFile(ctx context.Context, objectName string) error {
	obj := s.client.Bucket(s.bucketName).Object(objectName)
	if err := obj.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", objectName, err)
	}
	return nil
}

func (s *GCSStorage) SignedURL(ctx context.Context, objectName string, expires time.Duration) (string, error) {
	opts := &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(expires),
	}

	url, err := s.client.Bucket(s.bucketName).SignedURL(objectName, opts)
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return url, nil
}
