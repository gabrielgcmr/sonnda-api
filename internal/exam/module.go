package exam

import (
	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
)

type Module struct {
	Handler *Handler
}

func NewModule(storage *storage.Client, bucket string) *Module {
	StorageClient := NewGCSStorage(storage, bucket)
	svc := NewService(StorageClient)
	handle := NewHandler(svc)

	return &Module{
		Handler: handle,
	}

}

func (m *Module) SetupRoutes(rg *gin.RouterGroup) {
	Routes(rg, m.Handler)
}
