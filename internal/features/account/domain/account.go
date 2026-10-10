// internal/features/account/domain/account.go
package accountdomain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidAccountID   = errors.New("invalid account id")
	ErrInvalidAccountType = errors.New("invalid account type")
)

type Account struct {
	ID          uuid.UUID   `json:"id"`
	AccountType AccountType `json:"account_type"`
	Profile     Profile     `json:"profile"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	DeletedAt   *time.Time  `json:"deleted_at"`
}

type NewAccountParams struct {
	AccountType AccountType
	Profile     Profile
}

func NewAccount(params NewAccountParams) (*Account, error) {
	kind := params.AccountType.Normalize()
	if kind == "" {
		kind = AccountTypeBasicCare
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate account id: %w", err)
	}
	now := time.Now().UTC()
	a := &Account{ID: id, AccountType: kind, Profile: params.Profile.Normalize(), CreatedAt: now, UpdatedAt: now}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Account) Validate() error {
	if a == nil || a.ID == uuid.Nil {
		return ErrInvalidAccountID
	}
	if !a.AccountType.IsValid() {
		return ErrInvalidAccountType
	}
	return a.Profile.Validate()
}

// OnboardingCompleted is derived from the profile, independently of account status.
func (a *Account) OnboardingCompleted() bool {
	return a != nil && a.Profile.OnboardingCompleted()
}

// ApplyProfile replaces the profile only after validating every supplied value.
// A nil field clears its value. Equivalent profiles preserve UpdatedAt.
func (a *Account) ApplyProfile(profile Profile) (bool, error) {
	if a == nil {
		return false, ErrInvalidAccountID
	}
	next := profile.Normalize()
	if err := next.Validate(); err != nil {
		return false, err
	}
	if a.Profile.Equal(next) {
		return false, nil
	}
	a.Profile = next
	a.UpdatedAt = time.Now().UTC()
	return true, nil
}
