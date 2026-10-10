// internal/features/patient/problem/policy.go
package problem

import (
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// Action identifies one operation covered by the patient problem policy.
type Action string

const (
	ListProblems    Action = "list_problems"
	ReadProblem     Action = "read_problem"
	ReadHistory     Action = "read_problem_history"
	CreateProblem   Action = "create_problem"
	EditProblem     Action = "edit_problem"
	ClassifyProblem Action = "classify_problem"
	ResolveProblem  Action = "resolve_problem"
	ReopenProblem   Action = "reopen_problem"
	RectifyProblem  Action = "rectify_problem"
	MergeProblems   Action = "merge_problems"
)

// RequireAction applies problem-specific permissions to shared authorization facts.
func RequireAction(action Action, patientID uuid.UUID, actor authz.PatientContext) error {
	if actor.AccountID == uuid.Nil {
		return apperr.Unauthorized("autenticação necessária")
	}
	if patientID == uuid.Nil || actor.PatientID != patientID || !actor.HasAccess || !actor.AccountType.IsValid() {
		return apperr.Forbidden("acesso negado")
	}

	switch action {
	case ListProblems, ReadProblem, ReadHistory:
		return nil
	case CreateProblem, EditProblem, ClassifyProblem, ReopenProblem,
		RectifyProblem, MergeProblems:
		if actor.AccountType == accountdomain.AccountTypeProfessional {
			return nil
		}
	case ResolveProblem:
		return nil
	}
	return apperr.Forbidden("acesso negado")
}
