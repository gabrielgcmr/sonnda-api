// internal/api/routes.go
package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gabrielgcmr/sonnda/internal/api/middleware"
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	capturehttp "github.com/gabrielgcmr/sonnda/internal/features/capture/http"
	documentprocessinghttp "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/http"
	accesshttp "github.com/gabrielgcmr/sonnda/internal/features/patient/access/http"
	laboratoryhttp "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/http"
	problemhttp "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/http"
	profilehttp "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/http"
	"github.com/gabrielgcmr/sonnda/static"
	"github.com/gin-gonic/gin"
)

type APIDependencies struct {
	APIInfo                            APIInfo
	Auth                               *authhttp.Middleware
	Account                            *accounthttp.Middleware
	AccountHandler                     *accounthttp.Handler
	CaptureHandler                     *capturehttp.Handler
	CaptureAuth                        *capturehttp.Middleware
	CaptureCleanup                     *capturehttp.CleanupHandler
	PatientAccessHandler               *accesshttp.Handler
	PatientHandler                     *profilehttp.Handler
	PatientProblemHandler              *problemhttp.Handler
	LaboratoryHandler                  *laboratoryhttp.Handler
	ExamsHandler                       *documentprocessinghttp.ExamsHandler
	StandaloneLabExtractionHandler     *documentprocessinghttp.StandaloneLabExtractionHandler
	StandaloneImagingExtractionHandler *documentprocessinghttp.StandaloneImagingExtractionHandler
	TemporaryExtractionHandler         *documentprocessinghttp.TemporaryExtractionHandler
}

type routeGroups struct {
	public        huma.API
	authenticated huma.API
	resolved      huma.API
	onboarded     huma.API
	captureMobile huma.API
}

// SetupRoutes validates the runtime dependencies, registers the endpoints, and
// returns the configured Huma API.
func SetupRoutes(r *gin.Engine, deps *APIDependencies) (huma.API, error) {
	if err := validateAPIDependencies(deps); err != nil {
		return nil, err
	}

	r.GET("/favicon.ico", func(c *gin.Context) {
		c.Data(http.StatusOK, "image/x-icon", static.FaviconICO)
	})

	apiInfo := normalizedAPIInfo(deps.APIInfo)
	humaAPI := newHumaAPI(r, apiInfo)
	registerRuntimeRoutes(runtimeRouteGroups(humaAPI, deps), deps)
	return humaAPI, nil
}

func runtimeRouteGroups(api huma.API, deps *APIDependencies) routeGroups {
	authenticated := huma.NewGroup(api)
	authenticated.UseMiddleware(middleware.RequireBearer(api, deps.Auth))

	resolved := huma.NewGroup(authenticated)
	resolved.UseMiddleware(middleware.ResolveAccount(api, deps.Account))

	onboarded := huma.NewGroup(resolved)
	onboarded.UseMiddleware(middleware.RequireCompletedOnboarding(api))

	captureMobile := huma.NewGroup(api)
	captureMobile.UseMiddleware(middleware.RequireCaptureToken(api, deps.CaptureAuth))

	return routeGroups{
		public:        api,
		authenticated: authenticated,
		resolved:      resolved,
		onboarded:     onboarded,
		captureMobile: captureMobile,
	}
}

func specificationRouteGroups(api huma.API) routeGroups {
	return routeGroups{
		public:        huma.NewGroup(api),
		authenticated: huma.NewGroup(api),
		resolved:      huma.NewGroup(api),
		onboarded:     huma.NewGroup(api),
		captureMobile: huma.NewGroup(api),
	}
}

func registerRuntimeRoutes(groups routeGroups, deps *APIDependencies) {
	registerHealthRoute(groups.public)

	deps.AccountHandler.RegisterAuthenticatedRoutes(groups.authenticated, bearerSecurity())
	deps.AccountHandler.RegisterResolvedRoutes(groups.resolved, bearerSecurity())
	deps.CaptureHandler.RegisterPublicRoutes(groups.public)
	deps.CaptureHandler.RegisterMobileRoutes(groups.captureMobile, captureTokenSecurity())
	deps.CaptureHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())
	deps.CaptureCleanup.RegisterHumaRoutes(groups.public)

	deps.PatientAccessHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())
	deps.PatientHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())
	deps.PatientProblemHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())

	deps.ExamsHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())
	deps.StandaloneLabExtractionHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())
	deps.StandaloneImagingExtractionHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())
	deps.TemporaryExtractionHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())
	deps.LaboratoryHandler.RegisterHumaRoutes(groups.onboarded, bearerSecurity())
}

func registerOpenAPIRoutes(groups routeGroups) {
	registerHealthRoute(groups.public)

	accounthttp.RegisterAuthenticatedOpenAPIRoutes(groups.authenticated, bearerSecurity())
	accounthttp.RegisterResolvedOpenAPIRoutes(groups.resolved, bearerSecurity())
	capturehttp.RegisterPublicOpenAPIRoutes(groups.public)
	capturehttp.RegisterMobileOpenAPIRoutes(groups.captureMobile, captureTokenSecurity())
	capturehttp.RegisterOpenAPIRoutes(groups.onboarded, bearerSecurity())
	capturehttp.RegisterCleanupOpenAPIRoutes(groups.public)

	accesshttp.RegisterOpenAPIRoutes(groups.onboarded, bearerSecurity())
	profilehttp.RegisterOpenAPIRoutes(groups.onboarded, bearerSecurity())
	problemhttp.RegisterOpenAPIRoutes(groups.onboarded, bearerSecurity())

	documentprocessinghttp.RegisterExamsOpenAPIRoutes(groups.onboarded, bearerSecurity())
	documentprocessinghttp.RegisterStandaloneLabExtractionOpenAPIRoutes(groups.onboarded, bearerSecurity())
	documentprocessinghttp.RegisterStandaloneImagingExtractionOpenAPIRoutes(groups.onboarded, bearerSecurity())
	documentprocessinghttp.RegisterTemporaryExtractionOpenAPIRoutes(groups.onboarded, bearerSecurity())
	laboratoryhttp.RegisterOpenAPIRoutes(groups.onboarded, bearerSecurity())
}

func validateAPIDependencies(deps *APIDependencies) error {
	missing := make([]string, 0, 12)
	if deps == nil || deps.Auth == nil {
		missing = append(missing, "Auth")
	}
	if deps == nil || deps.Account == nil {
		missing = append(missing, "Account")
	}
	if deps == nil || deps.AccountHandler == nil {
		missing = append(missing, "AccountHandler")
	}
	if deps == nil || deps.CaptureHandler == nil {
		missing = append(missing, "CaptureHandler")
	}
	if deps == nil || deps.CaptureAuth == nil {
		missing = append(missing, "CaptureAuth")
	}
	if deps == nil || deps.CaptureCleanup == nil {
		missing = append(missing, "CaptureCleanup")
	}
	if deps == nil || deps.PatientAccessHandler == nil {
		missing = append(missing, "PatientAccessHandler")
	}
	if deps == nil || deps.PatientHandler == nil {
		missing = append(missing, "PatientHandler")
	}
	if deps == nil || deps.PatientProblemHandler == nil {
		missing = append(missing, "PatientProblemHandler")
	}
	if deps == nil || deps.LaboratoryHandler == nil {
		missing = append(missing, "LaboratoryHandler")
	}
	if deps == nil || deps.ExamsHandler == nil {
		missing = append(missing, "ExamsHandler")
	}
	if deps == nil || deps.StandaloneLabExtractionHandler == nil {
		missing = append(missing, "StandaloneLabExtractionHandler")
	}
	if deps == nil || deps.StandaloneImagingExtractionHandler == nil {
		missing = append(missing, "StandaloneImagingExtractionHandler")
	}
	if deps == nil || deps.TemporaryExtractionHandler == nil {
		missing = append(missing, "TemporaryExtractionHandler")
	}
	if len(missing) > 0 {
		return fmt.Errorf("dependências obrigatórias da API ausentes: %s", strings.Join(missing, ", "))
	}

	return nil
}

func bearerSecurity() []map[string][]string {
	return []map[string][]string{{bearerAuthScheme: {}}}
}

func captureTokenSecurity() []map[string][]string {
	return []map[string][]string{{captureTokenAuthScheme: {}}}
}
