// internal/features/account/postgres/repository_test.go
package accountpostgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accountsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/account"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreatePreservesNullsAndSeparateIdentity(t *testing.T) {
	a, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	i, _ := accountdomain.NewIdentity(a.ID, "issuer", "subject", nil)
	q := &stubQueries{}
	if err := (&Repository{queries: q}).Create(t.Context(), a, i); err != nil {
		t.Fatal(err)
	}
	p := q.created
	if p.ID != a.ID || p.Issuer != i.Issuer || p.Subject != i.Subject || p.Email.Valid ||
		p.FullName.Valid || p.BirthDate.Valid || p.Cpf.Valid || p.Phone.Valid ||
		!p.CreatedAt.Time.Equal(a.CreatedAt) || !p.IdentityCreatedAt.Time.Equal(i.CreatedAt) {
		t.Fatalf("unexpected create parameters: %+v", p)
	}
	i.AccountID = uuid.New()
	if err := (&Repository{queries: q}).Create(t.Context(), a, i); !errors.Is(err, persistence.ErrPersistenceFailure) {
		t.Fatal("mismatched account and identity IDs must fail")
	}
}

func TestLookupsPreserveOptionalProfileAndDeactivation(t *testing.T) {
	for _, lookup := range []struct {
		name string
		find func(*Repository) (*accountdomain.Account, error)
	}{
		{"id", func(r *Repository) (*accountdomain.Account, error) { return r.FindByID(t.Context(), uuid.New()) }},
		{"identity", func(r *Repository) (*accountdomain.Account, error) {
			return r.FindByAuthIdentity(t.Context(), "issuer", "subject")
		}},
		{"cpf", func(r *Repository) (*accountdomain.Account, error) { return r.FindByCPF(t.Context(), "12345678901") }},
	} {
		t.Run(lookup.name, func(t *testing.T) {
			row := accountsqlc.Account{ID: uuid.New(), AccountType: "basic_care", DeletedAt: timestamp(time.Now().UTC())}
			q := &stubQueries{row: row}
			r := &Repository{queries: q}
			a, err := lookup.find(r)
			if err != nil || a.ID != row.ID || a.Profile != (accountdomain.Profile{}) || a.DeletedAt == nil {
				t.Fatalf("unexpected account: %+v, %v", a, err)
			}
			q.err = fmt.Errorf("read: %w", pgx.ErrNoRows)
			if a, err := lookup.find(r); a != nil || err != nil {
				t.Fatalf("missing account: %+v %v", a, err)
			}
			failure := errors.New("database unavailable")
			q.err = failure
			if _, err := lookup.find(r); !errors.Is(err, persistence.ErrPersistenceFailure) || !errors.Is(err, failure) {
				t.Fatal(err)
			}
		})
	}
}

func TestIdentityLookupKeepsIdentityDataOutOfAccount(t *testing.T) {
	q := &stubQueries{identity: accountsqlc.AccountIdentity{AccountID: uuid.New(), Issuer: "issuer", Subject: "subject"}}
	r := &Repository{queries: q}
	i, err := r.FindIdentity(t.Context(), "issuer", "subject")
	if err != nil || i.AccountID != q.identity.AccountID || i.Issuer != "issuer" || i.Subject != "subject" || i.Email != nil {
		t.Fatalf("identity=%+v err=%v", i, err)
	}
	q.err = pgx.ErrNoRows
	if i, err := r.FindIdentity(t.Context(), "issuer", "subject"); i != nil || err != nil {
		t.Fatal("missing identity semantics changed")
	}
	q.err = errors.New("read failed")
	if _, err := r.FindIdentity(t.Context(), "issuer", "subject"); !errors.Is(err, persistence.ErrPersistenceFailure) || !errors.Is(err, q.err) {
		t.Fatal(err)
	}
}

func TestUpdatePreservesNullsAndMapsDatabaseValues(t *testing.T) {
	a, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	row := accountsqlc.Account{ID: a.ID, AccountType: "basic_care", FullName: pgtype.Text{String: "Ana Silva", Valid: true}, UpdatedAt: timestamp(a.UpdatedAt.Add(time.Second))}
	q := &stubQueries{row: row}
	if err := (&Repository{queries: q}).Update(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if q.updated.FullName.Valid || q.updated.BirthDate.Valid || q.updated.Cpf.Valid || q.updated.Phone.Valid {
		t.Fatalf("NULL profile was replaced with zero values: %+v", q.updated)
	}
	if a.Profile.FullName == nil || *a.Profile.FullName != "Ana Silva" || !a.UpdatedAt.Equal(row.UpdatedAt.Time) {
		t.Fatal("returned database values were not mapped")
	}
}

func TestWriteErrorsPreserveContractAndAccount(t *testing.T) {
	a, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	i, _ := accountdomain.NewIdentity(a.ID, "issuer", "subject", nil)
	for _, operation := range []struct {
		name string
		run  func(*Repository) error
	}{
		{"create", func(r *Repository) error { return r.Create(t.Context(), a, i) }},
		{"update", func(r *Repository) error { return r.Update(t.Context(), a) }},
		{"deactivate", func(r *Repository) error { return r.SoftDelete(t.Context(), a.ID) }},
		{"activate", func(r *Repository) error { _, err := r.ActivateProfessional(t.Context(), a.ID); return err }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			failure := errors.New("database unavailable")
			q := &stubQueries{err: failure}
			before := *a
			if err := operation.run(&Repository{queries: q}); !errors.Is(err, persistence.ErrPersistenceFailure) || !errors.Is(err, failure) || !reflect.DeepEqual(before, *a) {
				t.Fatalf("error=%v account=%+v", err, a)
			}
			q.err = &pgconn.PgError{Code: "23505"}
			if err := operation.run(&Repository{queries: q}); !errors.Is(err, account.ErrAccountAlreadyExists) {
				t.Fatal(err)
			}
			if operation.name != "create" {
				q.err = pgx.ErrNoRows
				if err := operation.run(&Repository{queries: q}); !errors.Is(err, account.ErrAccountNotFound) {
					t.Fatal(err)
				}
			}
		})
	}
}

type stubQueries struct {
	accountsqlc.Querier
	row      accountsqlc.Account
	identity accountsqlc.AccountIdentity
	err      error
	rows     int64
	created  accountsqlc.CreateAccountWithIdentityParams
	updated  accountsqlc.UpdateAccountProfileParams
}

func (q *stubQueries) CreateAccountWithIdentity(_ context.Context, p accountsqlc.CreateAccountWithIdentityParams) error {
	q.created = p
	return q.err
}
func (q *stubQueries) UpdateAccountProfile(_ context.Context, p accountsqlc.UpdateAccountProfileParams) (accountsqlc.Account, error) {
	q.updated = p
	return q.row, q.err
}
func (q *stubQueries) FindAccountByID(context.Context, uuid.UUID) (accountsqlc.Account, error) {
	return q.row, q.err
}
func (q *stubQueries) FindAccountByCPF(context.Context, pgtype.Text) (accountsqlc.Account, error) {
	return q.row, q.err
}
func (q *stubQueries) FindAccountByAuthIdentity(context.Context, accountsqlc.FindAccountByAuthIdentityParams) (accountsqlc.Account, error) {
	return q.row, q.err
}
func (q *stubQueries) FindAccountIdentity(context.Context, accountsqlc.FindAccountIdentityParams) (accountsqlc.AccountIdentity, error) {
	return q.identity, q.err
}
func (q *stubQueries) SoftDeleteAccount(context.Context, uuid.UUID) (int64, error) {
	return q.rows, q.err
}
func (q *stubQueries) ActivateAccountAsProfessional(context.Context, uuid.UUID) (accountsqlc.Account, error) {
	return q.row, q.err
}
