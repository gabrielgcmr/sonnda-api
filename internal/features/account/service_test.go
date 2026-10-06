// internal/features/account/service_test.go
package account

import (
	"context"
	"errors"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type profileRepository struct {
	Repository
	account   *accountdomain.Account
	identity  *accountdomain.Identity
	updates   int
	deleteErr error
}

func (r *profileRepository) Create(_ context.Context, a *accountdomain.Account, i *accountdomain.Identity) error {
	r.account, r.identity = a, i
	return nil
}
func (r *profileRepository) FindByID(context.Context, uuid.UUID) (*accountdomain.Account, error) {
	return r.account, nil
}
func (r *profileRepository) Update(context.Context, *accountdomain.Account) error {
	r.updates++
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
	if _, err := s.Update(t.Context(), AccountUpdateInput{AccountID: a.ID, FullName: &newName}); err != nil || r.updates != 1 || *a.Profile.Phone != phone {
		t.Fatalf("partial update=%+v error=%v", a, err)
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
