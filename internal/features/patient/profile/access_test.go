// internal/features/patient/profile/access_test.go
package patientprofile

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type deniedPatientAccess struct {
	err       error
	accountID uuid.UUID
	patientID uuid.UUID
}

func (a *deniedPatientAccess) RequireAccess(_ context.Context, accountID, patientID uuid.UUID) error {
	a.accountID, a.patientID = accountID, patientID
	return a.err
}

func TestPatientOperationsStopWhenAccessIsDenied(t *testing.T) {
	actor := &accountdomain.Account{ID: uuid.New()}
	patientID := uuid.New()
	denied := apperr.Forbidden("acesso negado")
	for _, operation := range []string{"get", "update", "soft delete", "hard delete"} {
		t.Run(operation, func(t *testing.T) {
			accessChecker := &deniedPatientAccess{err: denied}
			// Nil repositories ensure no read or write happens after denial.
			svc := New(nil, nil, accessChecker)
			var err error
			switch operation {
			case "get":
				_, err = svc.Get(context.Background(), actor, patientID)
			case "update":
				_, err = svc.Update(context.Background(), actor, patientID, UpdateInput{})
			case "soft delete":
				err = svc.SoftDelete(context.Background(), actor, patientID)
			case "hard delete":
				err = svc.HardDelete(context.Background(), actor, patientID)
			}
			if !errors.Is(err, denied) {
				t.Fatalf("expected access denial, got %v", err)
			}
			if accessChecker.accountID != actor.ID || accessChecker.patientID != patientID {
				t.Fatal("wrong access check")
			}
		})
	}
}
