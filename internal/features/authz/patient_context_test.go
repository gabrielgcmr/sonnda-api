// internal/features/authz/patient_context_test.go
package authz

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accessdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/access/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type contextAccounts struct{ user *accountdomain.User }

func (a contextAccounts) FindByID(context.Context, uuid.UUID) (*accountdomain.User, error) {
	return a.user, nil
}

type contextAccess struct{ err error }

func (a contextAccess) RequireAccess(context.Context, uuid.UUID, uuid.UUID) error { return a.err }

type contextRelationships struct {
	relation *accessdomain.RelationshipType
}

func (r contextRelationships) FindActiveRelationship(context.Context, uuid.UUID, uuid.UUID) (*accessdomain.RelationshipType, error) {
	return r.relation, nil
}

type contextIdentity struct {
	verified bool
	err      error
}

func (i contextIdentity) HasActiveSelf(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return i.verified, i.err
}

func TestPatientContextUsesVerifiedIdentityNotLegacyRelationship(t *testing.T) {
	accountID, patientID := uuid.New(), uuid.New()
	self := accessdomain.RelationshipTypeSelf
	resolver := NewPatientContextResolver(
		contextAccounts{user: &accountdomain.User{ID: accountID, AccountType: accountdomain.AccountTypeBasicCare}},
		contextAccess{}, contextRelationships{relation: &self}, contextIdentity{},
	)
	actor, err := resolver.Resolve(context.Background(), accountID, patientID)
	if err != nil {
		t.Fatal(err)
	}
	if actor.SelfVerified || actor.CaregiverAuthorized || actor.RelationshipType == nil || *actor.RelationshipType != self {
		t.Fatalf("legacy relationship must remain metadata: %+v", actor)
	}
	assertProblemAction(t, "unverified self", ResolveProblem, patientID, actor, false)
	resolver.selfIdentity = contextIdentity{verified: true}
	actor, err = resolver.Resolve(context.Background(), accountID, patientID)
	if err != nil {
		t.Fatal(err)
	}
	assertProblemAction(t, "professionally confirmed self", ResolveProblem, patientID, actor, true)
}

func TestPatientContextFailsClosed(t *testing.T) {
	accountID, patientID := uuid.New(), uuid.New()
	accounts := contextAccounts{user: &accountdomain.User{ID: accountID, AccountType: accountdomain.AccountTypeBasicCare}}
	resolver := NewPatientContextResolver(accounts, contextAccess{err: apperr.Forbidden("acesso negado")}, contextRelationships{}, contextIdentity{verified: true})
	if _, err := resolver.Resolve(context.Background(), accountID, patientID); contextErrorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("expected access denial: %v", err)
	}
	resolver.access = contextAccess{}
	resolver.selfIdentity = contextIdentity{err: errors.New("database unavailable")}
	if _, err := resolver.Resolve(context.Background(), accountID, patientID); contextErrorKind(err) != apperr.INFRA_DATABASE_ERROR {
		t.Fatalf("expected database error: %v", err)
	}
	resolver.accounts = contextAccounts{}
	if _, err := resolver.Resolve(context.Background(), accountID, patientID); contextErrorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("expected missing account denial: %v", err)
	}
}

func contextErrorKind(err error) apperr.ErrorKind {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return ""
}
