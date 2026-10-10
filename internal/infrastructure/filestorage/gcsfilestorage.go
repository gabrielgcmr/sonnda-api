// internal/infrastructure/filestorage/gcsfilestorage.go
package filestorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

type GCSObjectStorage struct {
	client     *storage.Client
	bucketName string
	projectID  string
}

var _ documentprocessing.FileStorageService = (*GCSObjectStorage)(nil)

func NewGCSObjectStorage(
	ctx context.Context,
	bucketName,
	projectID string,
	opts ...option.ClientOption) (*GCSObjectStorage, error) {
	var (
		client *storage.Client
		err    error
	)

	if len(opts) > 0 {
		client, err = storage.NewClient(ctx, opts...)
	} else {
		client, err = storage.NewClient(ctx)
	}
	if err != nil {
		return nil, wrapStorageError("falha ao inicializar storage", "gcs.new_client", err)
	}

	return &GCSObjectStorage{
		client:     client,
		bucketName: bucketName,
		projectID:  projectID,
	}, nil
}

func (a *GCSObjectStorage) Upload(
	ctx context.Context,
	file io.Reader,
	objectName string,
	contentType string) (string, error) {

	bucket := a.client.Bucket(a.bucketName)
	object := bucket.Object(objectName)
	writer := object.NewWriter(ctx)

	if contentType != "" {
		writer.ContentType = contentType
	}

	if _, err := io.Copy(writer, file); err != nil {
		_ = writer.Close()
		return "", wrapStorageError("falha ao enviar arquivo", "gcs.upload.copy", err)
	}
	if err := writer.Close(); err != nil {
		return "", wrapStorageError("falha ao enviar arquivo", "gcs.upload.close_writer", err)
	}

	// URL pública assinada (24h)
	gcsURI := fmt.Sprintf("gs://%s/%s", a.bucketName, objectName)
	return gcsURI, nil
}

func (a *GCSObjectStorage) Delete(ctx context.Context, uri string) error {
	objectName := extractObjectName(uri, a.bucketName)

	object := a.client.Bucket(a.bucketName).Object(objectName)
	if err := object.Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return wrapStorageError("falha ao remover arquivo", "gcs.delete", fmt.Errorf("uri=%s: %w", uri, err))
	}
	return nil
}

func (a *GCSObjectStorage) Open(ctx context.Context, uri string) (io.ReadCloser, error) {
	objectName := extractObjectName(uri, a.bucketName)
	reader, err := a.client.Bucket(a.bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return nil, wrapStorageError("falha ao abrir arquivo", "gcs.open", err)
	}
	return reader, nil
}

func (a *GCSObjectStorage) GetSignedURL(
	ctx context.Context,
	uri string,
	expirationMinutes int,
) (string, error) {
	objectName := extractObjectName(uri, a.bucketName)

	opts := &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(time.Duration(expirationMinutes) * time.Minute),
	}

	url, err := a.client.Bucket(a.bucketName).SignedURL(objectName, opts)
	if err != nil {
		return "", wrapStorageError("falha ao gerar URL assinada", "gcs.signed_url", err)
	}

	return url, nil
}

// Close libera o client subjacente do GCS.
func (a *GCSObjectStorage) Close() error {
	if a.client != nil {
		return a.client.Close()
	}
	return nil
}

// extractObjectName extrai o nome do objeto da URI
// Ex: "gs://bucket-name/path/to/file.pdf" -> "path/to/file.pdf"
func extractObjectName(uri, bucketName string) string {
	prefix := fmt.Sprintf("gs://%s/", bucketName)
	if len(uri) > len(prefix) && uri[:len(prefix)] == prefix {
		return uri[len(prefix):]
	}
	// Se não tiver o prefixo, assume que já é o object name
	return uri
}

func wrapStorageError(message, op string, err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled):
		return &apperr.AppError{
			Kind:    apperr.INFRA_TIMEOUT,
			Message: "tempo limite excedido",
			Cause:   fmt.Errorf("%s: %w", op, err),
		}

	case errors.Is(err, storage.ErrObjectNotExist):
		return &apperr.AppError{
			Kind:    apperr.NOT_FOUND,
			Message: "arquivo não encontrado",
			Cause:   fmt.Errorf("%s: %w", op, err),
		}

	default:
		return &apperr.AppError{
			Kind:    apperr.INFRA_STORAGE_ERROR,
			Message: message,
			Cause:   fmt.Errorf("%s: %w", op, err),
		}
	}
}
