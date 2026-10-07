// internal/features/account/service_test.go
package account

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type resolveRepository struct {
	Repository
	mu         sync.Mutex
	accounts   map[string]*accountdomain.Account
	identities map[string]*accountdomain.Identity
	creates    int
}

func newResolveRepository() *resolveRepository {
	return &resolveRepository{
		accounts:   make(map[string]*accountdomain.Account),
		identities: make(map[string]*accountdomain.Identity),
	}
}

func (r *resolveRepository) WithinTransaction(_ context.Context, fn func(Repository) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn(r)
}

func (r *resolveRepository) LockAuthIdentity(context.Context, string, string) error { return nil }

func (r *resolveRepository) FindByAuthIdentity(_ context.Context, issuer, subject string) (*accountdomain.Account, error) {
	return r.accounts[identityKey(issuer, subject)], nil
}

func (r *resolveRepository) Create(_ context.Context, a *accountdomain.Account, identity *accountdomain.Identity) error {
	key := identityKey(identity.Issuer, identity.Subject)
	r.accounts[key] = a
	r.identities[key] = identity
	r.creates++
	return nil
}

func (r *resolveRepository) UpdateIdentityEmail(_ context.Context, issuer, subject string, email *string) error {
	identity := r.identities[identityKey(issuer, subject)]
	if identity == nil {
		return ErrAccountNotFound
	}
	identity.Email = cloneString(email)
	return nil
}

func identityKey(issuer, subject string) string { return issuer + "\x00" + subject }

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type profileRepository struct {
	Repository
	mu        sync.Mutex
	account   *accountdomain.Account
	identity  *accountdomain.Identity
	updates   int
	deleteErr error
}

func (r *profileRepository) WithinTransaction(_ context.Context, fn func(Repository) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn(r)
}

func (r *profileRepository) Create(_ context.Context, a *accountdomain.Account, i *accountdomain.Identity) error {
	r.account, r.identity = a, i
	return nil
}
func (r *profileRepository) FindByID(context.Context, uuid.UUID) (*accountdomain.Account, error) {
	return r.account, nil
}
func (r *profileRepository) FindByIDForUpdate(context.Context, uuid.UUID) (*accountdomain.Account, error) {
	return r.account, nil
}
func (r *profileRepository) Update(_ context.Context, account *accountdomain.Account) error {
	r.updates++
	r.account = account
	return nil
}
func (r *profileRepository) SoftDelete(context.Context, uuid.UUID) error { return r.deleteErr }

func TestCreateKeepsAccountAndIdentitySeparate(t *testing.T) {
	r := &profileRepository{}
	a, err := New(r).Create(t.Context(), AccountCreateInput{Issuer: "issuer", Subject: "subject"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Profile != (accountdomain.Profile{}) || a.OnboardingCompleted() || r.identity.AccountID != a.ID || r.identity.Email != nil {
		t.Fatalf("account=%+v identity=%+v", a, r.identity)
	}
}

func TestResolveOrProvisionIsIdempotentAndSynchronizesEmail(t *testing.T) {
	repo := newResolveRepository()
	service := New(repo)
	firstEmail := "first@example.test"
	first, err := service.ResolveOrProvision(t.Context(), AccountResolveInput{
		Issuer: "issuer", Subject: "subject", Email: &firstEmail,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Profile != (accountdomain.Profile{}) || first.AccountType != accountdomain.AccountTypeBasicCare || repo.creates != 1 {
		t.Fatalf("unexpected provisioned account: %+v", first)
	}

	secondEmail := "second@example.test"
	second, err := service.ResolveOrProvision(t.Context(), AccountResolveInput{
		Issuer: "issuer", Subject: "subject", Email: &secondEmail,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity := repo.identities[identityKey("issuer", "subject")]
	if second.ID != first.ID || repo.creates != 1 || identity.Email == nil || *identity.Email != secondEmail {
		t.Fatalf("resolution was not idempotent: first=%s second=%s identity=%+v", first.ID, second.ID, identity)
	}

	other, err := service.ResolveOrProvision(t.Context(), AccountResolveInput{Issuer: "other-issuer", Subject: "subject"})
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == first.ID || repo.creates != 2 {
		t.Fatal("different issuers shared an account")
	}
}

func TestResolveOrProvisionRejectsDeactivatedAccount(t *testing.T) {
	repo := newResolveRepository()
	service := New(repo)
	created, err := service.ResolveOrProvision(t.Context(), AccountResolveInput{Issuer: "issuer", Subject: "subject"})
	if err != nil {
		t.Fatal(err)
	}
	deletedAt := time.Now().UTC()
	created.DeletedAt = &deletedAt

	resolved, err := service.ResolveOrProvision(t.Context(), AccountResolveInput{Issuer: "issuer", Subject: "subject"})
	var appErr *apperr.AppError
	if resolved != nil || !errors.As(err, &appErr) || appErr.Kind != apperr.ACCESS_DENIED || repo.creates != 1 {
		t.Fatalf("resolved=%+v error=%v creates=%d", resolved, err, repo.creates)
	}
}

func TestConcurrentResolutionReturnsOneAccount(t *testing.T) {
	repo := newResolveRepository()
	service := New(repo)
	const workers = 32
	ids := make(chan uuid.UUID, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			resolved, err := service.ResolveOrProvision(t.Context(), AccountResolveInput{Issuer: "issuer", Subject: "subject"})
			if err != nil {
				errs <- err
				return
			}
			ids <- resolved.ID
		}()
	}
	group.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var expected uuid.UUID
	for id := range ids {
		if expected == uuid.Nil {
			expected = id
		}
		if id != expected {
			t.Fatalf("concurrent resolution returned %s and %s", expected, id)
		}
	}
	if repo.creates != 1 {
		t.Fatalf("created %d accounts", repo.creates)
	}
}

func TestProfileUpdateValidatesBeforeWritingAndSkipsNoOp(t *testing.T) {
	name, phone := "Ana Silva", "11999999999"
	a, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{Profile: accountdomain.Profile{FullName: &name, Phone: &phone}})
	r := &profileRepository{account: a}
	s := New(r)
	before := *a
	if _, err := s.Update(t.Context(), AccountUpdateInput{AccountID: a.ID, FullName: &name}); err != nil || r.updates != 0 || !a.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("no-op wrote profile: %v", err)
	}
	newName, invalidPhone := "Novo Nome", "bad"
	_, err := s.Update(t.Context(), AccountUpdateInput{AccountID: a.ID, FullName: &newName, Phone: &invalidPhone})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Kind != apperr.VALIDATION_FAILED || r.updates != 0 || *a != before {
		t.Fatalf("invalid update=%+v error=%v", a, err)
	}
	updated, err := s.Update(t.Context(), AccountUpdateInput{AccountID: a.ID, FullName: &newName})
	if err != nil || r.updates != 1 || updated.Profile.Phone == nil || *updated.Profile.Phone != phone || *updated.Profile.FullName != newName {
		t.Fatalf("partial update=%+v error=%v", updated, err)
	}
}

func TestConcurrentProfileUpdatesPreserveDifferentFields(t *testing.T) {
	account, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	repo := &profileRepository{account: account}
	service := New(repo)
	name := "Ana Silva"
	phone := "11999999999"
	inputs := []AccountUpdateInput{
		{AccountID: account.ID, FullName: &name},
		{AccountID: account.ID, Phone: &phone},
	}
	errs := make(chan error, len(inputs))
	var group sync.WaitGroup
	for _, input := range inputs {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.Update(t.Context(), input)
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if repo.account.Profile.FullName == nil || *repo.account.Profile.FullName != name ||
		repo.account.Profile.Phone == nil || *repo.account.Profile.Phone != phone {
		t.Fatalf("concurrent update lost a field: %+v", repo.account.Profile)
	}
}

func TestDeactivatedAccountCannotBeUpdatedAndDeletionIsIdempotent(t *testing.T) {
	a, _ := accountdomain.NewAccount(accountdomain.NewAccountParams{})
	now := time.Now().UTC()
	a.DeletedAt = &now
	r := &profileRepository{account: a, deleteErr: ErrAccountNotFound}
	s := New(r)
	_, err := s.Update(t.Context(), AccountUpdateInput{AccountID: a.ID})
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Kind != apperr.ACCESS_DENIED || r.updates != 0 {
		t.Fatal(err)
	}
	if err := s.SoftDelete(t.Context(), a.ID); err != nil {
		t.Fatal(err)
	}
}
