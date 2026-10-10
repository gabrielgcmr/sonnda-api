// internal/features/patient/problem/domain/history.go
package problemdomain

import (
	"time"

	"github.com/google/uuid"
)

type Action string

const (
	ActionCreated           Action = "created"
	ActionEdited            Action = "edited"
	ActionClassified        Action = "classified"
	ActionResolved          Action = "resolved"
	ActionReopened          Action = "reopened"
	ActionRectified         Action = "rectified"
	ActionMergedSource      Action = "merged_source"
	ActionMergedDestination Action = "merged_destination"
)

// HistoryEvent is persisted together with the corresponding problem version.
// Before is nil only for creation. SourceProblemIDs records incorporated origins.
type HistoryEvent struct {
	ID               uuid.UUID
	ProblemID        uuid.UUID
	PatientID        uuid.UUID
	Version          int64
	Action           Action
	ActorAccountID   uuid.UUID
	OccurredAt       time.Time
	Before           *Snapshot
	After            Snapshot
	Reason           string
	SourceProblemIDs []uuid.UUID
}

func cloneSnapshot(s Snapshot) Snapshot {
	return s.normalized()
}
