// internal/features/authz/problem_policy.go
package authz

import (
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// ProblemAction identifies one operation covered by the patient problem policy.
type ProblemAction string

const (
	ListProblems    ProblemAction = "list_problems"
	ReadProblem     ProblemAction = "read_problem"
	ReadHistory     ProblemAction = "read_problem_history"
	CreateProblem   ProblemAction = "create_problem"
	EditProblem     ProblemAction = "edit_problem"
	ClassifyProblem ProblemAction = "classify_problem"
	ResolveProblem  ProblemAction = "resolve_problem"
	ReopenProblem   ProblemAction = "reopen_problem"
	RectifyProblem  ProblemAction = "rectify_problem"
	MergeProblems   ProblemAction = "merge_problems"
)

// PatientContext contains backend facts for one account and one patient.
// The caller must obtain these facts from the registered account and patient/access,
// never from the operation payload.
type PatientContext struct {
	AccountID   uuid.UUID
	PatientID   uuid.UUID
	AccountType accountdomain.AccountType
	HasAccess   bool
}

// RequireProblemAction applies actor permissions to the requested patient.
// Clinical and administrative rules, including classification, state transitions,
// rectification reason and same-patient merge validation, belong to patient/problem.
func RequireProblemAction(action ProblemAction, patientID uuid.UUID, actor PatientContext) error {
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
