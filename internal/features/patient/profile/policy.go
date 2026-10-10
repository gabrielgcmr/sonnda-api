// internal/features/patient/profile/policy.go
package patientprofile

import (
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// Action identifies one operation covered by the patient profile policy.
type Action string

const (
	ReadProfile       Action = "read_profile"
	UpdateProfile     Action = "update_profile"
	SoftDeleteProfile Action = "soft_delete_profile"
	HardDeleteProfile Action = "hard_delete_profile"
)

// RequireAction applies profile-specific permissions to shared authorization facts.
func RequireAction(action Action, patientID uuid.UUID, actor authz.PatientContext) error {
	if err := requireAccountAction(action, actor); err != nil {
		return err
	}
	if patientID == uuid.Nil || actor.PatientID != patientID || !actor.HasAccess {
		return apperr.Forbidden("acesso negado")
	}
	return nil
}

func requireAccountAction(action Action, actor authz.PatientContext) error {
	if actor.AccountID == uuid.Nil {
		return apperr.Unauthorized("autenticação necessária")
	}
	if !actor.AccountType.IsValid() {
		return apperr.Forbidden("acesso negado")
	}
	switch action {
	case ReadProfile, UpdateProfile:
		return nil
	case SoftDeleteProfile, HardDeleteProfile:
		if actor.AccountType == accountdomain.AccountTypeProfessional {
			return nil
		}
	}
	return apperr.Forbidden("acesso negado")
}
