// internal/features/account/postgres/repository.go
package accountpostgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	usersqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/user"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
)

var _ account.Repository = (*Repository)(nil)

type Repository struct {
	queries usersqlc.Querier
}

func New(db usersqlc.DBTX) *Repository {
	return &Repository{
		queries: usersqlc.New(db),
	}
}

// Create implements [account.Repository].
func (r *Repository) Create(ctx context.Context, u *accountdomain.User) error {
	params := usersqlc.CreateUserParams{
		ID:          u.ID,
		AuthIssuer:  u.AuthIssuer,
		AuthSubject: u.AuthSubject,
		Email:       u.Email,
		FullName:    u.FullName,
		BirthDate:   pgtype.Date{Time: u.BirthDate, Valid: true},
		Cpf:         u.CPF,
		Phone:       u.Phone,
		AccountType: string(u.AccountType),
		CreatedAt:   pgtype.Timestamptz{Time: u.CreatedAt, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: u.UpdatedAt, Valid: true},
	}

	if err := r.queries.CreateUser(ctx, params); err != nil {
		if isUniqueViolation(err) {
			return account.ErrUserAlreadyExists
		}
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}

	// NÇœo sobrescreve a entidade; app-source-of-truth mantÇ¸m valores do domÇðnio.
	return nil
}

// SoftDelete implements [account.Repository].
func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.SoftDeleteUser(ctx, id)
	if err != nil {
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}
	if rows == 0 {
		return account.ErrUserNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.DeleteUser(ctx, id)
	if err != nil {
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}
	if rows == 0 {
		return account.ErrUserNotFound
	}
	return nil
}

// FindByAuthIdentity implements [account.Repository].
func (r *Repository) FindByAuthIdentity(ctx context.Context, issuer string, subject string) (*accountdomain.User, error) {

	row, err := r.queries.FindUserByAuthIdentity(ctx, usersqlc.FindUserByAuthIdentityParams{
		AuthIssuer:  issuer,
		AuthSubject: subject,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}

	return &accountdomain.User{
		ID:          row.ID,
		AuthIssuer:  row.AuthIssuer,
		AuthSubject: row.AuthSubject,
		Email:       row.Email,
		FullName:    row.FullName,
		BirthDate:   row.BirthDate.Time,
		CPF:         row.Cpf,
		Phone:       row.Phone,
		AccountType: accountdomain.AccountType(row.AccountType),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

// FindByCPF implements [account.Repository].
func (r *Repository) FindByCPF(ctx context.Context, cpf string) (*accountdomain.User, error) {
	row, err := r.queries.FindUserByCPF(ctx, cpf)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}

	return &accountdomain.User{
		ID:          row.ID,
		AuthIssuer:  row.AuthIssuer,
		AuthSubject: row.AuthSubject,
		Email:       row.Email,
		FullName:    row.FullName,
		BirthDate:   row.BirthDate.Time,
		CPF:         row.Cpf,
		Phone:       row.Phone,
		AccountType: accountdomain.AccountType(row.AccountType),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

// FindByID implements [account.Repository].
func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (*accountdomain.User, error) {
	row, err := r.queries.FindUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}

	return &accountdomain.User{
		ID:          row.ID,
		AuthIssuer:  row.AuthIssuer,
		AuthSubject: row.AuthSubject,
		Email:       row.Email,
		FullName:    row.FullName,
		BirthDate:   row.BirthDate.Time,
		CPF:         row.Cpf,
		Phone:       row.Phone,
		AccountType: accountdomain.AccountType(row.AccountType),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

// Update implements [account.Repository].
func (r *Repository) Update(ctx context.Context, u *accountdomain.User) error {
	row, err := r.queries.UpdateUser(ctx, usersqlc.UpdateUserParams{
		ID:        u.ID,
		Email:     u.Email,
		FullName:  u.FullName,
		BirthDate: pgtype.Date{Time: u.BirthDate, Valid: true},
		Cpf:       u.CPF,
		Phone:     u.Phone,
		UpdatedAt: pgtype.Timestamptz{Time: u.UpdatedAt, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return account.ErrUserNotFound
		}
		if isUniqueViolation(err) {
			return account.ErrUserAlreadyExists
		}
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}

	u.ID = row.ID
	u.Email = row.Email
	u.FullName = row.FullName
	u.BirthDate = row.BirthDate.Time
	u.CPF = row.Cpf
	u.Phone = row.Phone
	u.UpdatedAt = row.UpdatedAt.Time

	return nil
}

func (r *Repository) ActivateProfessional(ctx context.Context, id uuid.UUID) (*accountdomain.User, error) {
	row, err := r.queries.ActivateUserAsProfessional(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, account.ErrUserNotFound
		}
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}
	return &accountdomain.User{
		ID: row.ID, AuthIssuer: row.AuthIssuer, AuthSubject: row.AuthSubject,
		Email: row.Email, FullName: row.FullName, BirthDate: row.BirthDate.Time,
		CPF: row.Cpf, Phone: row.Phone, AccountType: accountdomain.AccountType(row.AccountType),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*accountdomain.User, error) {
	row, err := r.queries.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}

	return &accountdomain.User{
		ID:          row.ID,
		AuthIssuer:  row.AuthIssuer,
		AuthSubject: row.AuthSubject,
		Email:       row.Email,
		FullName:    row.FullName,
		BirthDate:   row.BirthDate.Time,
		CPF:         row.Cpf,
		Phone:       row.Phone,
		AccountType: accountdomain.AccountType(row.AccountType),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
