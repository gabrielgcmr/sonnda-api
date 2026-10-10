// internal/api/middleware/client_origin.go
package middleware

import (
	"net"
	"strings"

	"github.com/gabrielgcmr/sonnda/internal/api/helpers"
	"github.com/gin-gonic/gin"
)

// ClientOrigin records the direct peer IP. Using RemoteAddr avoids trusting
// forwarding headers until the deployment has an explicit trusted-proxy list.
func ClientOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.Request.RemoteAddr)
		if host, _, err := net.SplitHostPort(origin); err == nil {
			origin = host
		}
		ctx := helpers.ContextWithClientOrigin(c.Request.Context(), origin)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
