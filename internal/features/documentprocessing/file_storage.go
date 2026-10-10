// internal/features/documentprocessing/file_storage.go
package documentprocessing

import (
	"context"
	"io"
	"time"
)

const MaxFileSizeBytes int64 = 5 * 1024 * 1024

// FileStorageService define operações de armazenamento de arquivos.
type FileStorageService interface {
	Upload(ctx context.Context, file io.Reader, objectName, contentType string) (string, error)
	Delete(ctx context.Context, uri string) error
	GetSignedURL(ctx context.Context, uri string, expiresIn time.Duration) (string, error)
}
