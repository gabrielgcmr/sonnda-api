// internal/api/routes.go
package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/middleware"
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	capturehttp "github.com/gabrielgcmr/sonnda/internal/features/capture/http"
	documentprocessinghttp "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/http"
	accesshttp "github.com/gabrielgcmr/sonnda/internal/features/patient/access/http"
	laboratoryhttp "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/http"
	patienthttp "github.com/gabrielgcmr/sonnda/internal/features/patient/http"
	problemhttp "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/http"
	profilehttp "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/http"
	"github.com/gabrielgcmr/sonnda/static"
	"github.com/gin-gonic/gin"
)

type APIDependencies struct {
	APIInfo                        APIInfo
	Auth                           *authhttp.Middleware
	Account                        *accounthttp.Middleware
	AccountHandler                 *accounthttp.Handler
	CaptureHandler                 *capturehttp.Handler
	PatientAccessHandler           *accesshttp.Handler
	PatientCreationHandler         *patienthttp.CreationHandler
	PatientHandler                 *profilehttp.Handler
	PatientProblemHandler          *problemhttp.Handler
	LaboratoryHandler              *laboratoryhttp.Handler
	ExamsHandler                   *documentprocessinghttp.ExamsHandler
	StandaloneLabExtractionHandler *documentprocessinghttp.StandaloneLabExtractionHandler
}

// SetupRoutes registers the endpoints and returns the configured Huma API.
func SetupRoutes(r *gin.Engine, deps *APIDependencies) huma.API {
	r.GET("/favicon.ico", func(c *gin.Context) {
		c.Data(http.StatusOK, "image/x-icon", static.FaviconICO)
	})

	apiInfo := normalizedAPIInfo(deps.APIInfo)
	humaAPI := newHumaAPI(r, apiInfo)
	registerHumaRoutes(humaAPI, deps)
	return humaAPI
}

func registerHumaRoutes(api huma.API, deps *APIDependencies) {
	registerHealthRoute(api)

	authenticated := huma.NewGroup(api)
	authenticated.UseMiddleware(middleware.RequireBearer(api, deps.Auth))

	resolved := huma.NewGroup(authenticated)
	resolved.UseMiddleware(middleware.ResolveAccount(api, deps.Account))

	onboarded := huma.NewGroup(resolved)
	onboarded.UseMiddleware(middleware.RequireCompletedOnboarding(api))

	deps.AccountHandler.RegisterAuthenticatedRoutes(authenticated, bearerSecurity())
	deps.AccountHandler.RegisterResolvedRoutes(resolved, bearerSecurity())
	deps.AccountHandler.RegisterOnboardedRoutes(onboarded, bearerSecurity())
	deps.CaptureHandler.RegisterHumaRoutes(onboarded, bearerSecurity())

	deps.PatientAccessHandler.RegisterHumaRoutes(onboarded, bearerSecurity())
	deps.PatientCreationHandler.RegisterHumaRoutes(onboarded, bearerSecurity())
	deps.PatientHandler.RegisterHumaRoutes(onboarded, bearerSecurity())
	deps.PatientProblemHandler.RegisterHumaRoutes(onboarded, bearerSecurity())

	deps.ExamsHandler.RegisterHumaRoutes(onboarded, bearerSecurity())
	deps.StandaloneLabExtractionHandler.RegisterHumaRoutes(onboarded, bearerSecurity())
	deps.LaboratoryHandler.RegisterHumaRoutes(onboarded, bearerSecurity())
}

func bearerSecurity() []map[string][]string {
	return []map[string][]string{{bearerAuthScheme: {}}}
}
