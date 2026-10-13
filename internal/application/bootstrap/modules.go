// internal/application/bootstrap/modules.go
package bootstrap

import (
	"github.com/gabrielgcmr/sonnda/internal/config"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/redis/go-redis/v9"
)

type Modules struct {
	Account       *AccountModule
	Capture       *CaptureModule
	Patient       *PatientModule
	PatientAccess *PatientAccessModule
	Labs          *LabsModule
	Exams         *ExamsModule
}

func NewModules(
	dbClient *postgress.Client,
	redisClient *redis.Client,
	labExtractor labreport.LabReportExtractor,
	imagingExtractor imagingreport.Extractor,
	storage documentprocessing.FileStorageService,
	captureStorage capture.FileStorage,
	temporaryStorage documentprocessing.TemporaryFileStorage,
	ocrConfig config.OCRConfig,
	activationConfig config.ProfessionalActivationConfig,
	captureCleanupToken string,
) *Modules {
	captureModule := NewCaptureModule(dbClient, captureStorage, captureCleanupToken)
	examsModule := NewExamsModule(dbClient, labExtractor, imagingExtractor, storage, temporaryStorage, captureModule.Service, ocrConfig)
	captureModule.Cleanup.SetTemporaryCleaner(examsModule.TemporaryResults)
	return &Modules{
		Account:       NewAccountModule(dbClient, redisClient, activationConfig),
		Capture:       captureModule,
		Patient:       NewPatientModule(dbClient),
		PatientAccess: NewPatientAccessModule(dbClient),
		Labs:          NewLabsModule(dbClient),
		Exams:         examsModule,
	}
}
