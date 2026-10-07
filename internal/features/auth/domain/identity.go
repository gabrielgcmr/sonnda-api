// internal/features/auth/domain/identity.go
package authdomain

// Identity representa o usuário autenticado (OIDC-compatible).
type Identity struct {
	Issuer  string
	Subject string

	Email         *string
	EmailVerified *bool

	Name       *string
	PictureURL *string

	Scopes []string
}

// NewIdentity preserves the issuer and subject provided by the authentication provider.
func NewIdentity(issuer, subject string) Identity {
	return Identity{
		Issuer:  issuer,
		Subject: subject,
	}
}
