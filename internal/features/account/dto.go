// internal/features/account/dto.go
package account

import (
	"time"

	"github.com/google/uuid"
)

// AccountResolveInput identifies the authenticated principal whose local
// account must be resolved or provisioned.
type AccountResolveInput struct {
	Issuer  string
	Subject string
	Email   *string
}

// OptionalField distinguishes an omitted patch field from an explicit null or
// concrete value. Set=false preserves the stored value; Set=true with a nil
// Value clears it.
type OptionalField[T any] struct {
	Set   bool
	Value *T
}

type AccountUpdateInput struct {
	AccountID uuid.UUID
	FullName  OptionalField[string]
	BirthDate OptionalField[time.Time]
	CPF       OptionalField[string]
	Phone     OptionalField[string]
}
