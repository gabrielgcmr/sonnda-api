// internal/application/bootstrap/capture.go
package bootstrap

import (
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturehttp "github.com/gabrielgcmr/sonnda/internal/features/capture/http"
	capturepostgres "github.com/gabrielgcmr/sonnda/internal/features/capture/postgres"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
)

type CaptureModule struct {
	Handler    *capturehttp.Handler
	Middleware *capturehttp.Middleware
}

func NewCaptureModule(db *postgress.Client, storage capture.FileStorage) *CaptureModule {
	repository := capturepostgres.NewRepository(db)
	service := capture.New(repository, storage)
	return &CaptureModule{
		Handler:    capturehttp.NewHandler(service),
		Middleware: capturehttp.NewMiddleware(service),
	}
}
