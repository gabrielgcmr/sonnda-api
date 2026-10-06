// internal/features/account/onboarding_dto.go
package account

import (
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
)

type RegisterInput struct {
	Issuer      string
	Subject     string
	Email       *string
	AccountType accountdomain.AccountType
	Profile     accountdomain.Profile
}
