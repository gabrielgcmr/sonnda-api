// internal/api/routes.go
package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/middleware"
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
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

	registered := huma.NewGroup(authenticated)
	registered.UseMiddleware(middleware.RequireRegisteredAccount(api, deps.Account))

	deps.AccountHandler.RegisterHumaRoutes(authenticated, registered, bearerSecurity())
	deps.PatientAccessHandler.RegisterHumaRoutes(registered, bearerSecurity())
	deps.PatientCreationHandler.RegisterHumaRoutes(registered, bearerSecurity())
	deps.PatientHandler.RegisterHumaRoutes(registered, bearerSecurity())
	deps.PatientProblemHandler.RegisterHumaRoutes(registered, bearerSecurity())
	deps.ExamsHandler.RegisterHumaRoutes(registered, bearerSecurity())
	deps.StandaloneLabExtractionHandler.RegisterHumaRoutes(registered, bearerSecurity())
	deps.LaboratoryHandler.RegisterHumaRoutes(registered, bearerSecurity())
}

func bearerSecurity() []map[string][]string {
	return []map[string][]string{{bearerAuthScheme: {}}}
}
