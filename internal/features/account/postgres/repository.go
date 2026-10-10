// internal/features/account/postgres/repository.go
package accountpostgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accountsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/account"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
)

var _ account.Repository = (*Repository)(nil)

type transactionDB interface {
	accountsqlc.DBTX
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

type Repository struct {
	db      transactionDB
	queries accountsqlc.Querier
}

func New(db transactionDB) *Repository {
	return &Repository{db: db, queries: accountsqlc.New(db)}
}

func (r *Repository) WithinTransaction(ctx context.Context, fn func(account.Repository) error) error {
	if r == nil || r.db == nil {
		return errors.Join(persistence.ErrPersistenceFailure, errors.New("transaction database is not configured"))
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	txRepo := &Repository{queries: accountsqlc.New(tx)}
	if err := fn(txRepo); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Join(persistence.ErrPersistenceFailure, err)
	}
	return nil
}

func (r *Repository) LockAuthIdentity(ctx context.Context, issuer, subject string) error {
	return readWriteError(r.queries.LockAccountIdentity(ctx, accountsqlc.LockAccountIdentityParams{
		Issuer: issuer, Subject: subject,
	}))
}

func (r *Repository) Create(ctx context.Context, a *accountdomain.Account, identity *accountdomain.Identity) error {
	if a == nil || identity == nil || a.ID != identity.AccountID {
		return errors.Join(persistence.ErrPersistenceFailure, errors.New("account and identity ids must match"))
	}
	err := r.queries.CreateAccountWithIdentity(ctx, accountsqlc.CreateAccountWithIdentityParams{
		ID: a.ID, AccountType: string(a.AccountType),
		FullName: textValue(a.Profile.FullName), BirthDate: dateValue(a.Profile.BirthDate),
		Cpf: textValue(a.Profile.CPF), Phone: textValue(a.Profile.Phone),
		CreatedAt: timestamp(a.CreatedAt), UpdatedAt: timestamp(a.UpdatedAt),
		Issuer: identity.Issuer, Subject: identity.Subject, Email: textValue(identity.Email),
		IdentityCreatedAt: timestamp(identity.CreatedAt), IdentityUpdatedAt: timestamp(identity.UpdatedAt),
	})
	return writeError(err)
}

func (r *Repository) Update(ctx context.Context, a *accountdomain.Account) error {
	row, err := r.queries.UpdateAccountProfile(ctx, accountsqlc.UpdateAccountProfileParams{
		ID: a.ID, FullName: textValue(a.Profile.FullName), BirthDate: dateValue(a.Profile.BirthDate),
		Cpf: textValue(a.Profile.CPF), Phone: textValue(a.Profile.Phone), UpdatedAt: timestamp(a.UpdatedAt),
	})
	if err != nil {
		return writeError(err)
	}
	*a = *accountFromRow(row)
	return nil
}

func (r *Repository) UpdateIdentityEmail(ctx context.Context, issuer, subject string, email *string) error {
	_, err := r.queries.UpdateAccountIdentityEmail(ctx, accountsqlc.UpdateAccountIdentityEmailParams{
		Issuer: issuer, Subject: subject, Email: textValue(email),
	})
	return readWriteError(err)
}

func (r *Repository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.SoftDeleteAccount(ctx, id)
	if err != nil {
		return writeError(err)
	}
	if rows == 0 {
		return account.ErrAccountNotFound
	}
	return nil
}

func (r *Repository) ActivateProfessional(ctx context.Context, id uuid.UUID) (*accountdomain.Account, error) {
	row, err := r.queries.ActivateAccountAsProfessional(ctx, id)
	if err != nil {
		return nil, writeError(err)
	}
	return accountFromRow(row), nil
}

func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (*accountdomain.Account, error) {
	return accountResult(r.queries.FindAccountByID(ctx, id))
}

func (r *Repository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*accountdomain.Account, error) {
	return accountResult(r.queries.FindAccountByIDForUpdate(ctx, id))
}

func (r *Repository) FindByCPF(ctx context.Context, cpf string) (*accountdomain.Account, error) {
	return accountResult(r.queries.FindAccountByCPF(ctx, pgtype.Text{String: cpf, Valid: true}))
}

func (r *Repository) FindByAuthIdentity(ctx context.Context, issuer, subject string) (*accountdomain.Account, error) {
	return accountResult(r.queries.FindAccountByAuthIdentity(ctx, accountsqlc.FindAccountByAuthIdentityParams{
		Issuer: issuer, Subject: subject,
	}))
}

func (r *Repository) FindIdentity(ctx context.Context, issuer, subject string) (*accountdomain.Identity, error) {
	row, err := r.queries.FindAccountIdentity(ctx, accountsqlc.FindAccountIdentityParams{Issuer: issuer, Subject: subject})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}
	return &accountdomain.Identity{
		AccountID: row.AccountID, Issuer: row.Issuer, Subject: row.Subject, Email: textPointer(row.Email),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func accountResult(row accountsqlc.Account, err error) (*accountdomain.Account, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Join(persistence.ErrPersistenceFailure, err)
	}
	return accountFromRow(row), nil
}

func accountFromRow(row accountsqlc.Account) *accountdomain.Account {
	a := &accountdomain.Account{
		ID: row.ID, AccountType: accountdomain.AccountType(row.AccountType),
		Profile:   accountdomain.Profile{FullName: textPointer(row.FullName), CPF: textPointer(row.Cpf), Phone: textPointer(row.Phone)},
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.BirthDate.Valid {
		a.Profile.BirthDate = &row.BirthDate.Time
	}
	if row.DeletedAt.Valid {
		a.DeletedAt = &row.DeletedAt.Time
	}
	return a
}

func writeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return account.ErrAccountNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return account.ErrAccountAlreadyExists
	}
	return errors.Join(persistence.ErrPersistenceFailure, err)
}

func readWriteError(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(persistence.ErrPersistenceFailure, err)
}

func textValue(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func dateValue(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *value, Valid: true}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
