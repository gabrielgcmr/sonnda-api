// internal/api/test_dependencies_test.go
package api

import (
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	capturehttp "github.com/gabrielgcmr/sonnda/internal/features/capture/http"
	documentprocessinghttp "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/http"
	accesshttp "github.com/gabrielgcmr/sonnda/internal/features/patient/access/http"
	laboratoryhttp "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/http"
	problemhttp "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/http"
	profilehttp "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/http"
)

func completeTestDependencies() *APIDependencies {
	return &APIDependencies{
		Auth:                           authenticatedTestMiddleware(),
		Account:                        accounthttp.NewMiddleware(nil),
		AccountHandler:                 accounthttp.NewHandler(nil, nil),
		CaptureHandler:                 capturehttp.NewHandler(nil),
		CaptureAuth:                    capturehttp.NewMiddleware(nil),
		CaptureCleanup:                 capturehttp.NewCleanupHandler(nil, "test-cleanup-token"),
		PatientAccessHandler:           accesshttp.NewHandler(nil),
		PatientHandler:                 profilehttp.NewHandler(nil, nil),
		PatientProblemHandler:          problemhttp.NewHandler(nil),
		LaboratoryHandler:              laboratoryhttp.NewHandler(nil),
		ExamsHandler:                   documentprocessinghttp.NewExams(nil, nil, nil, nil),
		StandaloneLabExtractionHandler: documentprocessinghttp.NewStandaloneLabExtraction(nil),
	}
}
