// internal/application/bootstrap/exams.go
package bootstrap

import (
	"context"

	"github.com/gabrielgcmr/sonnda/internal/application/usecase/labdocumentconfirmation"
	"github.com/gabrielgcmr/sonnda/internal/config"
	accountpostgres "github.com/gabrielgcmr/sonnda/internal/features/account/postgres"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	documents "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	documenthttp "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/http"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	documentpostgres "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/postgres"
	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	accesspostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/access/postgres"
	labpostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/postgres"
	patientpostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/postgres"
	pginfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	pdftext "github.com/gabrielgcmr/sonnda/internal/infrastructure/pdftotext"
	"github.com/google/uuid"
)

type ExamsModule struct {
	Handler                            *documenthttp.ExamsHandler
	StandaloneLabExtractionHandler     *documenthttp.StandaloneLabExtractionHandler
	StandaloneImagingExtractionHandler *documenthttp.StandaloneImagingExtractionHandler
	TemporaryExtractionHandler         *documenthttp.TemporaryExtractionHandler
	TemporaryResults                   *documentprocessing.TemporaryResults
	TemporaryWorker                    *documentprocessing.TemporaryWorker
}

func NewExamsModule(db *pginfra.Client, lab labreport.LabReportExtractor, imaging imagingreport.Extractor, storage documentprocessing.FileStorageService, temporaryStorage documentprocessing.TemporaryFileStorage, captures capture.Service, config config.OCRConfig) *ExamsModule {
	patients := patientpostgres.NewRepository(db)
	labs := labpostgres.NewRepository(db)
	documentRepo := documentpostgres.NewDocumentRepository(db)
	access := patientaccess.NewChecker(patients, accesspostgres.NewRepository(db))
	authorizer := documents.NewAuthorizer(documentRepo, access)
	service := documents.New(documentRepo, authorizer)
	repo := documentpostgres.NewDraftRepository(db, labs)
	reader := pdftext.NewExtractorWithOptions(pdftext.ExtractorOptions{Timeout: config.Timeout, RequireUsableText: true})
	extractor := labreport.New(reader, lab)
	imagingService := imagingreport.New(reader, imaging)
	drafts := documents.NewDrafts(repo, extractor, storage, authorizer)
	confirmer := labdocumentconfirmation.New(authorizer, repo, labs)
	standalone := documents.NewStandaloneLabExtraction(extractor, authorizer)
	standaloneImaging := documents.NewStandaloneImagingExtraction(imagingService, authorizer)
	temporaryResults := documents.NewTemporaryResults(documentpostgres.NewTemporaryResultRepository(db), temporaryStorage)
	temporaryResults.SetCaptureSource(captures)
	captureSource := capturedFileSource{captures: captures}
	return &ExamsModule{
		Handler:                            documenthttp.NewExams(service, drafts, confirmer, storage),
		StandaloneLabExtractionHandler:     documenthttp.NewStandaloneLabExtraction(standalone, captureSource),
		StandaloneImagingExtractionHandler: documenthttp.NewStandaloneImagingExtraction(standaloneImaging, captureSource),
		TemporaryExtractionHandler:         documenthttp.NewTemporaryExtractionHandler(standalone, standaloneImaging, captureSource, temporaryResults),
		TemporaryResults:                   temporaryResults,
		TemporaryWorker:                    documents.NewTemporaryWorker(temporaryResults, accountpostgres.New(db.Pool()), captures, standalone, standaloneImaging),
	}
}

type capturedFileSource struct {
	captures captureFileSource
}

type captureFileSource interface {
	OpenCaptureFile(ctx context.Context, accountID, captureID uuid.UUID) (*capture.CaptureFile, error)
}

func (s capturedFileSource) OpenCapturedFile(ctx context.Context, accountID, captureID uuid.UUID) (*documenthttp.CapturedFile, error) {
	file, err := s.captures.OpenCaptureFile(ctx, accountID, captureID)
	if err != nil {
		return nil, err
	}
	return &documenthttp.CapturedFile{
		Body: file.Body, Filename: file.OriginalFilename, MIMEType: file.MIMEType, SizeBytes: file.SizeBytes, ExpiresAt: file.ExpiresAt,
	}, nil
}
