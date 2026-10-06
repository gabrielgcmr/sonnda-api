// internal/features/account/domain/identity.go
package accountdomain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidAuthIssuer  = errors.New("invalid auth issuer")
	ErrInvalidAuthSubject = errors.New("invalid auth subject")
)

// Identity is the persisted association between an authenticated subject and an account.
// Issuer and subject are opaque identifiers; email is optional provider information.
type Identity struct {
	AccountID uuid.UUID `json:"account_id"`
	Issuer    string    `json:"issuer"`
	Subject   string    `json:"subject"`
	Email     *string   `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewIdentity(accountID uuid.UUID, issuer, subject string, email *string) (*Identity, error) {
	now := time.Now().UTC()
	i := &Identity{AccountID: accountID, Issuer: issuer, Subject: subject, Email: trimmed(email, true), CreatedAt: now, UpdatedAt: now}
	if err := i.Validate(); err != nil {
		return nil, err
	}
	return i, nil
}

func (i *Identity) Validate() error {
	if i == nil || i.AccountID == uuid.Nil {
		return ErrInvalidAccountID
	}
	if strings.TrimSpace(i.Issuer) == "" {
		return ErrInvalidAuthIssuer
	}
	if strings.TrimSpace(i.Subject) == "" {
		return ErrInvalidAuthSubject
	}
	return nil
}
