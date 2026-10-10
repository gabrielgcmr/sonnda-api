// internal/api/openapi.go
package api

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gabrielgcmr/sonnda/internal/api/humaerror"
	"github.com/gin-gonic/gin"
)

// OpenAPI creates the Huma specification without connecting to infrastructure.
// Route registration only captures operation metadata; handlers are never run.
func OpenAPI(info APIInfo) *huma.OpenAPI {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	apiInfo := normalizedAPIInfo(info)
	humaAPI := newHumaAPI(router, apiInfo)
	registerHumaRoutes(humaAPI, &APIDependencies{})
	return humaAPI.OpenAPI()
}

const (
	bearerAuthScheme       = "bearerAuth"
	captureTokenAuthScheme = "captureTokenAuth"
)

type APIInfo struct {
	Name    string
	Version string
}

func normalizedAPIInfo(info APIInfo) APIInfo {
	name := info.Name
	if name == "" {
		name = "Sonnda API"
	}
	version := info.Version
	if version == "" {
		version = "dev"
	}

	return APIInfo{Name: name, Version: version}
}

func newHumaAPI(r *gin.Engine, info APIInfo) huma.API {
	config := huma.DefaultConfig(info.Name, info.Version)
	config.Transformers = append(config.Transformers, humaerror.Transform)
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		bearerAuthScheme: {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
		},
		captureTokenAuthScheme: {
			Type: "apiKey",
			In:   "header",
			Name: "X-Capture-Token",
		},
	}

	return humagin.New(r, config)
}
