// internal/api/helpers/client_origin.go
package helpers

import "context"

type clientOriginContextKey struct{}

func ContextWithClientOrigin(ctx context.Context, origin string) context.Context {
	return context.WithValue(ctx, clientOriginContextKey{}, origin)
}

func GetClientOriginFromContext(ctx context.Context) string {
	origin, _ := ctx.Value(clientOriginContextKey{}).(string)
	return origin
}
