// internal/application/bootstrap/modules.go
package bootstrap

import (
	"github.com/gabrielgcmr/sonnda/internal/config"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labextraction"
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
	labTextExtractor labextraction.LabReportTextExtractor,
	storage documentprocessing.FileStorageService,
	captureStorage capture.FileStorage,
	ocrConfig config.OCRConfig,
	activationConfig config.ProfessionalActivationConfig,
	captureCleanupToken string,
) *Modules {
	return &Modules{
		Account:       NewAccountModule(dbClient, redisClient, activationConfig),
		Capture:       NewCaptureModule(dbClient, captureStorage, captureCleanupToken),
		Patient:       NewPatientModule(dbClient),
		PatientAccess: NewPatientAccessModule(dbClient),
		Labs:          NewLabsModule(dbClient),
		Exams:         NewExamsModule(dbClient, labTextExtractor, storage, ocrConfig),
	}
}
