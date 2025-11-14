package exam

import (
	"context"

	"cloud.google.com/go/storage"
)

type Module struct {
	Handler *Handler
}

func NewModule(ctx context.Context, bucket string) (*Module, error) {
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	StorageClient := NewGCSStorage(gcsClient, bucket)
	svc := NewService(StorageClient)
	handle := NewHandler(svc)

	return &Module{
		Handler: handle,
	}, nil

}
