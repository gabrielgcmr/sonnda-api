// internal/api/middleware/client_origin_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gin-gonic/gin"
)

func TestClientOriginUsesDirectPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ClientOrigin())
	var origin string
	router.GET("/", func(c *gin.Context) {
		origin = helpers.GetClientOriginFromContext(c.Request.Context())
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:4321"
	request.Header.Set("X-Forwarded-For", "203.0.113.99")
	router.ServeHTTP(httptest.NewRecorder(), request)
	if origin != "192.0.2.10" {
		t.Fatalf("origin=%q, want direct peer", origin)
	}
}
