// internal/features/patient/problem/domain/problem.go
package problemdomain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Problem struct {
	ID                 uuid.UUID
	PatientID          uuid.UUID
	State              Snapshot
	CreatedByAccountID uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Version            int64
}

type NewProblemParams struct {
	PatientID      uuid.UUID
	ActorAccountID uuid.UUID
	Name           string
	CID11          *CID11
	Classification Classification
	OccurredAt     time.Time
}

// NewProblem requires an explicit classification and always starts active and valid.
// The caller authorizes the professional before invoking this domain operation.
func NewProblem(params NewProblemParams) (Problem, HistoryEvent, error) {
	if params.PatientID == uuid.Nil {
		return Problem{}, HistoryEvent{}, ErrInvalidPatientID
	}
	if params.ActorAccountID == uuid.Nil {
		return Problem{}, HistoryEvent{}, ErrInvalidActorID
	}
	if params.OccurredAt.IsZero() {
		return Problem{}, HistoryEvent{}, ErrInvalidTimestamp
	}
	when := params.OccurredAt.UTC()
	problem := Problem{
		ID:                 uuid.New(),
		PatientID:          params.PatientID,
		State:              Snapshot{Name: params.Name, CID11: params.CID11, Classification: params.Classification, ClinicalStatus: ClinicalStatusActive, AdministrativeStatus: AdministrativeStatusValid}.normalized(),
		CreatedByAccountID: params.ActorAccountID,
		CreatedAt:          when,
		UpdatedAt:          when,
		Version:            1,
	}
	if err := problem.Validate(); err != nil {
		return Problem{}, HistoryEvent{}, err
	}
	return problem, HistoryEvent{
		ID: uuid.New(), ProblemID: problem.ID, PatientID: problem.PatientID,
		Version: 1, Action: ActionCreated, ActorAccountID: params.ActorAccountID,
		OccurredAt: when, After: cloneSnapshot(problem.State),
	}, nil
}

func (p Problem) Validate() error {
	if p.ID == uuid.Nil {
		return ErrInvalidID
	}
	if p.PatientID == uuid.Nil {
		return ErrInvalidPatientID
	}
	if p.CreatedByAccountID == uuid.Nil {
		return ErrInvalidActorID
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() || p.UpdatedAt.Before(p.CreatedAt) {
		return ErrInvalidTimestamp
	}
	if p.Version < 1 {
		return ErrInvalidVersion
	}
	return p.State.Validate(p.ID)
}

type ChangeParams struct {
	Action           Action
	After            Snapshot
	ActorAccountID   uuid.UUID
	OccurredAt       time.Time
	Reason           string
	SourceProblemIDs []uuid.UUID
}

// Change returns a new version and its audit event; persistence must save both
// atomically and reject a stale version. Authorization stays in authz.
func (p Problem) Change(params ChangeParams) (Problem, HistoryEvent, error) {
	if err := p.Validate(); err != nil {
		return Problem{}, HistoryEvent{}, err
	}
	if params.ActorAccountID == uuid.Nil {
		return Problem{}, HistoryEvent{}, ErrInvalidActorID
	}
	if params.OccurredAt.IsZero() || params.OccurredAt.Before(p.UpdatedAt) {
		return Problem{}, HistoryEvent{}, ErrInvalidTimestamp
	}
	after := params.After.normalized()
	if err := after.Validate(p.ID); err != nil {
		return Problem{}, HistoryEvent{}, err
	}
	reason := strings.TrimSpace(params.Reason)
	if err := validateTransition(p.ID, p.State, after, params.Action, reason, params.SourceProblemIDs); err != nil {
		return Problem{}, HistoryEvent{}, err
	}
	before := cloneSnapshot(p.State)
	next := p
	next.State = cloneSnapshot(after)
	next.Version++
	next.UpdatedAt = params.OccurredAt.UTC()
	return next, HistoryEvent{
		ID: uuid.New(), ProblemID: p.ID, PatientID: p.PatientID,
		Version: next.Version, Action: params.Action, ActorAccountID: params.ActorAccountID,
		OccurredAt: next.UpdatedAt, Before: &before, After: cloneSnapshot(after),
		Reason: reason, SourceProblemIDs: append([]uuid.UUID(nil), params.SourceProblemIDs...),
	}, nil
}
