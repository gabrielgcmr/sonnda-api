// internal/features/account/repository.go
package account

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"

	"github.com/google/uuid"
)

var (
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUserNotFound      = errors.New("user not found")
)

// Repository persists account profiles. Lookups return (nil, nil) when absent;
// updates and deletions return ErrUserNotFound when the target no longer exists.
// Infrastructure failures wrap persistence.ErrPersistenceFailure and their cause.
type Repository interface {
	// CRUD basico
	Create(ctx context.Context, u *accountdomain.User) error
	Update(ctx context.Context, u *accountdomain.User) error
	ActivateProfessional(ctx context.Context, id uuid.UUID) (*accountdomain.User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	SoftDelete(ctx context.Context, id uuid.UUID) error

	// Buscas por atributos do usuario

	FindByID(ctx context.Context, id uuid.UUID) (*accountdomain.User, error)
	FindByEmail(ctx context.Context, email string) (*accountdomain.User, error)
	FindByCPF(ctx context.Context, cpf string) (*accountdomain.User, error)
	FindByAuthIdentity(ctx context.Context, issuer string, subject string) (*accountdomain.User, error)
}
