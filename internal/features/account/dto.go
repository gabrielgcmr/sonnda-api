// internal/features/account/dto.go
package account

import (
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"

	"github.com/google/uuid"
)

type AccountCreateInput struct {
	Issuer      string
	Subject     string
	Email       *string
	AccountType accountdomain.AccountType
	Profile     accountdomain.Profile
}

// AccountResolveInput identifies the authenticated principal whose local
// account must be resolved or provisioned.
type AccountResolveInput struct {
	Issuer  string
	Subject string
	Email   *string
}

type AccountUpdateInput struct {
	AccountID uuid.UUID
	FullName  *string
	BirthDate *time.Time
	CPF       *string
	Phone     *string
}
