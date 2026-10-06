// internal/features/account/repository.go
package account

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"

	"github.com/google/uuid"
)

var (
	ErrAccountAlreadyExists = errors.New("account already exists")
	ErrAccountNotFound      = errors.New("account not found")
)

// Repository persists account profiles. Lookups return (nil, nil) when absent;
// updates and deletions return ErrAccountNotFound when the target no longer exists.
// Infrastructure failures wrap persistence.ErrPersistenceFailure and their cause.
type Repository interface {
	// Create persists the account and its initial identity atomically.
	Create(ctx context.Context, a *accountdomain.Account, identity *accountdomain.Identity) error
	Update(ctx context.Context, u *accountdomain.Account) error
	ActivateProfessional(ctx context.Context, id uuid.UUID) (*accountdomain.Account, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error

	// Identity and ID lookups include deactivated accounts.
	FindByID(ctx context.Context, id uuid.UUID) (*accountdomain.Account, error)
	FindByCPF(ctx context.Context, cpf string) (*accountdomain.Account, error)
	FindByAuthIdentity(ctx context.Context, issuer string, subject string) (*accountdomain.Account, error)
	FindIdentity(ctx context.Context, issuer string, subject string) (*accountdomain.Identity, error)
}
