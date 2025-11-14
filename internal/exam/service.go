package exam

import (
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"time"
)

type Service struct {
	storage StorageClient
}

func NewService(storage StorageClient) *Service {
	return &Service{storage}
}

func (s *Service) UploadFile(ctx context.Context, file *multipart.FileHeader) (string, error) {
	ext := filepath.Ext(file.Filename)
	name := fmt.Sprintf("labs/%d%s", time.Now().UnixNano(), ext)

	url, err := s.storage.UploadFile(ctx, file, name)
	if err != nil {
		return "", err
	}

	return url, nil
}
