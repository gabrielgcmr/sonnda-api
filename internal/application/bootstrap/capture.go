// internal/application/bootstrap/capture.go
package bootstrap

import (
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturehttp "github.com/gabrielgcmr/sonnda/internal/features/capture/http"
	capturepostgres "github.com/gabrielgcmr/sonnda/internal/features/capture/postgres"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
)

type CaptureModule struct {
	Handler *capturehttp.Handler
}

func NewCaptureModule(db *postgress.Client) *CaptureModule {
	repository := capturepostgres.NewRepository(db)
	return &CaptureModule{Handler: capturehttp.NewHandler(capture.New(repository))}
}
