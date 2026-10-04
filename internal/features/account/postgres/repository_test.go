// internal/features/account/postgres/repository_test.go
package accountpostgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	usersqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/user"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreatePreservesProfileAndDateParameters(t *testing.T) {
	profile := testProfile()
	before := *profile
	queries := &stubQueries{}
	repo := &Repository{queries: queries}
	if err := repo.Create(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	params := queries.created
	if params.ID != profile.ID || params.AuthIssuer != profile.AuthIssuer || params.AuthSubject != profile.AuthSubject ||
		params.Email != profile.Email || params.FullName != profile.FullName || params.Cpf != profile.CPF ||
		params.Phone != profile.Phone || params.AccountType != string(profile.AccountType) {
		t.Fatalf("unexpected create parameters: %+v", params)
	}
	if !params.BirthDate.Valid || !params.BirthDate.Time.Equal(profile.BirthDate) ||
		!params.CreatedAt.Valid || !params.CreatedAt.Time.Equal(profile.CreatedAt) ||
		!params.UpdatedAt.Valid || !params.UpdatedAt.Time.Equal(profile.UpdatedAt) {
		t.Fatalf("dates must remain present and unchanged: %+v", params)
	}
	if *profile != before {
		t.Fatal("create must not overwrite the domain entity")
	}
}

func TestLookupsPreserveProfileAndMissingResultSemantics(t *testing.T) {
	profile := testProfile()
	for _, lookup := range []struct {
		name string
		find func(*Repository) (*accountdomain.User, error)
	}{
		{"identity", func(r *Repository) (*accountdomain.User, error) {
			return r.FindByAuthIdentity(t.Context(), profile.AuthIssuer, profile.AuthSubject)
		}},
		{"id", func(r *Repository) (*accountdomain.User, error) { return r.FindByID(t.Context(), profile.ID) }},
		{"cpf", func(r *Repository) (*accountdomain.User, error) { return r.FindByCPF(t.Context(), profile.CPF) }},
		{"email", func(r *Repository) (*accountdomain.User, error) { return r.FindByEmail(t.Context(), profile.Email) }},
	} {
		t.Run(lookup.name, func(t *testing.T) {
			queries := &stubQueries{row: profileRow(profile)}
			repo := &Repository{queries: queries}
			found, err := lookup.find(repo)
			if err != nil || found == nil || *found != *profile {
				t.Fatalf("profile = %+v, error = %v", found, err)
			}

			queries.err = fmt.Errorf("query: %w", pgx.ErrNoRows)
			if found, err := lookup.find(repo); found != nil || err != nil {
				t.Fatalf("absent profile = %+v, error = %v; want nil, nil", found, err)
			}

			queries.err = errors.New("database unavailable")
			found, err = lookup.find(repo)
			if found != nil || !errors.Is(err, persistence.ErrPersistenceFailure) || !errors.Is(err, queries.err) {
				t.Fatalf("lookup failure must preserve the category and cause: %v", err)
			}
		})
	}
}

func TestUpdateUsesReturnedDatabaseValues(t *testing.T) {
	profile := testProfile()
	row := profileRow(profile)
	row.FullName = "Updated name"
	row.UpdatedAt.Time = row.UpdatedAt.Time.Add(time.Second)
	queries := &stubQueries{row: row}
	if err := (&Repository{queries: queries}).Update(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	params := queries.updated
	if params.ID != profile.ID || params.FullName != "Ana Silva" || params.Cpf != profile.CPF ||
		!params.BirthDate.Valid || !params.BirthDate.Time.Equal(profile.BirthDate) || !params.UpdatedAt.Valid {
		t.Fatalf("unexpected update parameters: %+v", params)
	}
	if profile.FullName != row.FullName || !profile.UpdatedAt.Equal(row.UpdatedAt.Time) || profile.AuthSubject != row.AuthSubject {
		t.Fatalf("unexpected updated entity: %+v", profile)
	}
}

func TestActivateProfessionalUsesAtomicQueryAndMapsErrors(t *testing.T) {
	profile := testProfile()
	row := profileRow(profile)
	row.AccountType = string(accountdomain.AccountTypeProfessional)
	queries := &stubQueries{row: row}
	repository := &Repository{queries: queries}

	activated, err := repository.ActivateProfessional(t.Context(), profile.ID)
	if err != nil || activated == nil || activated.AccountType != accountdomain.AccountTypeProfessional {
		t.Fatalf("activation result=%+v error=%v", activated, err)
	}
	queries.err = fmt.Errorf("activate: %w", pgx.ErrNoRows)
	if _, err := repository.ActivateProfessional(t.Context(), profile.ID); !errors.Is(err, account.ErrUserNotFound) {
		t.Fatalf("missing account error=%v", err)
	}
	failure := errors.New("database unavailable")
	queries.err = failure
	if _, err := repository.ActivateProfessional(t.Context(), profile.ID); !errors.Is(err, persistence.ErrPersistenceFailure) || !errors.Is(err, failure) {
		t.Fatalf("persistence error=%v", err)
	}
}

func TestWriteErrorsPreserveRepositoryContract(t *testing.T) {
	unique := fmt.Errorf("query: %w", &pgconn.PgError{Code: "23505"})
	failure := errors.New("database unavailable")
	for _, operation := range []struct {
		name  string
		write func(*Repository, *accountdomain.User) error
	}{
		{"create", func(r *Repository, u *accountdomain.User) error { return r.Create(t.Context(), u) }},
		{"update", func(r *Repository, u *accountdomain.User) error { return r.Update(t.Context(), u) }},
		{"delete", func(r *Repository, u *accountdomain.User) error { return r.Delete(t.Context(), u.ID) }},
		{"soft delete", func(r *Repository, u *accountdomain.User) error { return r.SoftDelete(t.Context(), u.ID) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			queries := &stubQueries{err: failure}
			repo := &Repository{queries: queries}
			profile := testProfile()
			before := *profile
			err := operation.write(repo, profile)
			if !errors.Is(err, persistence.ErrPersistenceFailure) || !errors.Is(err, failure) || *profile != before {
				t.Fatalf("failure must preserve cause and entity: %v", err)
			}

			if operation.name == "create" || operation.name == "update" {
				queries.err = unique
				if err := operation.write(repo, profile); !errors.Is(err, account.ErrUserAlreadyExists) {
					t.Fatalf("unique violation = %v", err)
				}
			}
			if operation.name == "create" {
				return
			}
			queries.err = nil
			if operation.name == "update" {
				queries.err = fmt.Errorf("query: %w", pgx.ErrNoRows)
			}
			if err := operation.write(repo, profile); !errors.Is(err, account.ErrUserNotFound) {
				t.Fatalf("missing target = %v", err)
			}
			if operation.name != "update" {
				queries.rows = 1
				if err := operation.write(repo, profile); err != nil {
					t.Fatalf("successful deletion = %v", err)
				}
			}
		})
	}
}

func testProfile() *accountdomain.User {
	return &accountdomain.User{
		ID: uuid.New(), AuthIssuer: "test", AuthSubject: "subject-1", Email: "ana@example.test",
		FullName: "Ana Silva", AccountType: accountdomain.AccountTypeBasicCare, CPF: "12345678901", Phone: "11999999999",
		BirthDate: time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
	}
}

func profileRow(u *accountdomain.User) usersqlc.User {
	return usersqlc.User{
		ID: u.ID, AuthIssuer: u.AuthIssuer, AuthSubject: u.AuthSubject, Email: u.Email,
		FullName: u.FullName, AccountType: string(u.AccountType), Cpf: u.CPF, Phone: u.Phone,
		BirthDate: pgtype.Date{Time: u.BirthDate, Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: u.CreatedAt, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: u.UpdatedAt, Valid: true},
	}
}

type stubQueries struct {
	row     usersqlc.User
	err     error
	rows    int64
	created usersqlc.CreateUserParams
	updated usersqlc.UpdateUserParams
}

func (q *stubQueries) ActivateUserAsProfessional(context.Context, uuid.UUID) (usersqlc.User, error) {
	return q.row, q.err
}

func (q *stubQueries) CreateUser(_ context.Context, params usersqlc.CreateUserParams) error {
	q.created = params
	return q.err
}

func (q *stubQueries) UpdateUser(_ context.Context, params usersqlc.UpdateUserParams) (usersqlc.User, error) {
	q.updated = params
	return q.row, q.err
}

func (q *stubQueries) DeleteUser(context.Context, uuid.UUID) (int64, error) {
	return q.rows, q.err
}

func (q *stubQueries) SoftDeleteUser(context.Context, uuid.UUID) (int64, error) {
	return q.rows, q.err
}

func (q *stubQueries) FindUserByAuthIdentity(context.Context, usersqlc.FindUserByAuthIdentityParams) (usersqlc.User, error) {
	return q.row, q.err
}

func (q *stubQueries) FindUserByID(context.Context, uuid.UUID) (usersqlc.User, error) {
	return q.row, q.err
}

func (q *stubQueries) FindUserByCPF(context.Context, string) (usersqlc.User, error) {
	return q.row, q.err
}

func (q *stubQueries) FindUserByEmail(context.Context, string) (usersqlc.User, error) {
	return q.row, q.err
}
