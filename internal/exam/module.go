package exam

import (
	"cloud.google.com/go/storage"
)

type Module struct {
	Handler *Handler
}

func NewModule(gcsClient *storage.Client) *Module {
	bucket := "sonnda.firebasestorage.app"

	storage := NewGCSStorage(gcsClient, bucket)
	svc := NewService(storage)
	handle := NewHandler(svc)

	return &Module{
		Handler: handle,
	}

}
