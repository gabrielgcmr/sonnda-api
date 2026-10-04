// internal/features/patient/access/self_identity_test.go
package patientaccess

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type identityAccounts map[uuid.UUID]*accountdomain.User

func (a identityAccounts) FindByID(_ context.Context, id uuid.UUID) (*accountdomain.User, error) {
	return a[id], nil
}

type identityAccess struct{ err error }

func (a identityAccess) RequireAccess(_ context.Context, _, _ uuid.UUID) error { return a.err }

type identityStore struct {
	confirmations int
	revocations   int
	err           error
}

func (s *identityStore) HasActiveSelf(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}
func (s *identityStore) ConfirmSelf(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	s.confirmations++
	return s.err
}
func (s *identityStore) RevokeSelf(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	s.revocations++
	return s.err
}

func TestSelfIdentityRequiresDifferentProfessionalWithAccess(t *testing.T) {
	professionalID, targetID, patientID := uuid.New(), uuid.New(), uuid.New()
	accounts := identityAccounts{
		professionalID: {ID: professionalID, AccountType: accountdomain.AccountTypeProfessional},
		targetID:       {ID: targetID, AccountType: accountdomain.AccountTypeBasicCare},
	}
	store := &identityStore{}
	svc := NewSelfIdentityService(accounts, identityAccess{}, store)
	if err := svc.Confirm(context.Background(), professionalID, patientID, targetID); err != nil {
		t.Fatal(err)
	}
	if store.confirmations != 1 {
		t.Fatal("expected confirmation to reach the repository")
	}
	if err := svc.Confirm(context.Background(), professionalID, patientID, professionalID); errorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("self confirmation should be denied: %v", err)
	}
	if err := svc.Confirm(context.Background(), targetID, patientID, professionalID); errorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("basic care account should be denied: %v", err)
	}
	noAccess := NewSelfIdentityService(accounts, identityAccess{err: apperr.Forbidden("acesso negado")}, store)
	if err := noAccess.Confirm(context.Background(), professionalID, patientID, targetID); errorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("professional without patient access should be denied: %v", err)
	}
	if store.confirmations != 1 {
		t.Fatal("denied actors reached persistence")
	}
	if err := svc.Revoke(context.Background(), professionalID, patientID, professionalID); err != nil {
		t.Fatalf("professional may revoke their own verified link: %v", err)
	}
}

func TestSelfIdentityMapsPersistenceResults(t *testing.T) {
	professionalID, targetID, patientID := uuid.New(), uuid.New(), uuid.New()
	accounts := identityAccounts{
		professionalID: {ID: professionalID, AccountType: accountdomain.AccountTypeProfessional},
		targetID:       {ID: targetID, AccountType: accountdomain.AccountTypeBasicCare},
	}
	store := &identityStore{err: ErrSelfIdentityConflict}
	svc := NewSelfIdentityService(accounts, identityAccess{}, store)
	if err := svc.Confirm(context.Background(), professionalID, patientID, targetID); errorKind(err) != apperr.RESOURCE_CONFLICT {
		t.Fatalf("expected conflict: %v", err)
	}
	store.err = ErrSelfIdentityAbsent
	if err := svc.Revoke(context.Background(), professionalID, patientID, targetID); errorKind(err) != apperr.NOT_FOUND {
		t.Fatalf("expected absent confirmation: %v", err)
	}
	store.err = errors.New("database unavailable")
	if err := svc.Confirm(context.Background(), professionalID, patientID, targetID); errorKind(err) != apperr.INFRA_DATABASE_ERROR {
		t.Fatalf("expected database error: %v", err)
	}
}

func errorKind(err error) apperr.ErrorKind {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return ""
}
