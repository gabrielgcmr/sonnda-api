package exam

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"

	"cloud.google.com/go/storage"
)

type StorageClient interface {
	UploadFile(ctx context.Context, file *multipart.FileHeader, objectName string) (string, error)
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
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	// URL pública assinada (24h)
	url := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.bucketName, objectName)
	return url, nil
}
