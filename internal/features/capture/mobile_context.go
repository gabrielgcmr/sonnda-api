// internal/features/capture/mobile_context.go
package capture

import "context"

type mobileCredentialContextKey struct{}

func ContextWithMobileCredential(ctx context.Context, credential MobileCredential) context.Context {
	return context.WithValue(ctx, mobileCredentialContextKey{}, credential)
}

func MobileCredentialFromContext(ctx context.Context) (MobileCredential, bool) {
	credential, ok := ctx.Value(mobileCredentialContextKey{}).(MobileCredential)
	return credential, ok
}
