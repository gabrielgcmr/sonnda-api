// internal/features/patient/exam/laboratory/policy.go
package laboratory

import (
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type Action string

const (
	ListPatientReports Action = "list_patient_reports"
	ReadReport         Action = "read_report"
)

func RequireAction(action Action, patientID uuid.UUID, actor authz.PatientContext) error {
	if actor.AccountID == uuid.Nil {
		return apperr.Unauthorized("autenticação necessária")
	}
	if !actor.AccountType.IsValid() || patientID == uuid.Nil || actor.PatientID != patientID || !actor.HasAccess {
		return apperr.Forbidden("acesso negado")
	}
	switch action {
	case ListPatientReports, ReadReport:
		return nil
	default:
		return apperr.Forbidden("acesso negado")
	}
}

func patientAction(action Action) bool {
	return action == ListPatientReports || action == ReadReport
}
