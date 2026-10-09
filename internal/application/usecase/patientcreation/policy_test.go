// internal/application/usecase/patientcreation/policy_test.go
package patientcreation

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accessdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/access/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

func TestRequireInitialRelationship(t *testing.T) {
	relationships := []accessdomain.RelationshipType{
		accessdomain.RelationshipTypeSelf,
		accessdomain.RelationshipTypeFamily,
		accessdomain.RelationshipTypeCaregiver,
		accessdomain.RelationshipTypeProfessional,
	}
	accountTypes := []accountdomain.AccountType{
		accountdomain.AccountTypeBasicCare,
		accountdomain.AccountTypeProfessional,
	}

	for _, accountType := range accountTypes {
		for _, relationship := range relationships {
			name := string(accountType) + "/" + string(relationship)
			t.Run(name, func(t *testing.T) {
				err := RequireInitialRelationship(creatorAccount(accountType), relationship)
				forbidden := accountType == accountdomain.AccountTypeBasicCare &&
					relationship == accessdomain.RelationshipTypeProfessional
				if !forbidden && err != nil {
					t.Fatalf("expected relationship to be allowed, got %v", err)
				}
				if forbidden && appErrorKind(err) != apperr.ACCESS_DENIED {
					t.Fatalf("expected access denied, got %v", err)
				}
			})
		}
	}
}

func TestExecuteRejectsProfessionalRelationshipForBasicCareAccount(t *testing.T) {
	repo := &creationRepository{}

	_, err := New(repo).Execute(
		context.Background(),
		creatorAccount(accountdomain.AccountTypeBasicCare),
		validInput(string(accessdomain.RelationshipTypeProfessional)),
	)

	if appErrorKind(err) != apperr.ACCESS_DENIED {
		t.Fatalf("expected access denied, got %v", err)
	}
	if repo.patient != nil || repo.access != nil {
		t.Fatal("forbidden relationship must stop persistence")
	}
}

func appErrorKind(err error) apperr.ErrorKind {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return ""
}
