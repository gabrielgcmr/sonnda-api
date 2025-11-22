package infra

import (
	"context"

	"cloud.google.com/go/storage"
)

func NewGCSClient() (*storage.Client, error) {
	ctx := context.Background()
	return storage.NewClient(ctx)
}
