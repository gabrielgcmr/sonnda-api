// internal/features/patient/problem/http/response.go
package problemhttp

import (
	"time"

	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
)

type ProblemCID11 struct {
	Code    string `json:"code" minLength:"1"`
	System  string `json:"system" minLength:"1"`
	Version string `json:"version" minLength:"1"`
}

type ProblemSnapshot struct {
	Name                 string        `json:"name"`
	CID11                *ProblemCID11 `json:"cid11,omitempty"`
	Classification       string        `json:"classification" enum:"acute,chronic"`
	ClinicalStatus       string        `json:"clinical_status" enum:"active,resolved"`
	AdministrativeStatus string        `json:"administrative_status" enum:"valid,merged,entered_in_error"`
	MergedIntoID         *uuid.UUID    `json:"merged_into_id,omitempty" format:"uuid"`
}

type ProblemResponse struct {
	ProblemSnapshot
	ID                 uuid.UUID `json:"id" format:"uuid"`
	PatientID          uuid.UUID `json:"patient_id" format:"uuid"`
	CreatedByAccountID uuid.UUID `json:"created_by_account_id" format:"uuid"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Version            int64     `json:"version"`
}

type ProblemHistoryEvent struct {
	ID               uuid.UUID        `json:"id" format:"uuid"`
	ProblemID        uuid.UUID        `json:"problem_id" format:"uuid"`
	PatientID        uuid.UUID        `json:"patient_id" format:"uuid"`
	Version          int64            `json:"version"`
	Action           string           `json:"action" enum:"created,edited,classified,resolved,reopened,rectified,merged_source,merged_destination"`
	ActorAccountID   uuid.UUID        `json:"actor_account_id" format:"uuid"`
	OccurredAt       time.Time        `json:"occurred_at"`
	BeforeSnapshot   *ProblemSnapshot `json:"before_snapshot"`
	AfterSnapshot    ProblemSnapshot  `json:"after_snapshot"`
	Reason           string           `json:"reason,omitempty"`
	SourceProblemIDs []uuid.UUID      `json:"source_problem_ids"`
}

type ProblemPage struct {
	Items   []ProblemResponse `json:"items"`
	Limit   int               `json:"limit"`
	Offset  int               `json:"offset"`
	HasMore bool              `json:"has_more"`
}

type ProblemHistoryPage struct {
	Items   []ProblemHistoryEvent `json:"items"`
	Limit   int                   `json:"limit"`
	Offset  int                   `json:"offset"`
	HasMore bool                  `json:"has_more"`
}

func snapshotResponse(s problemdomain.Snapshot) ProblemSnapshot {
	result := ProblemSnapshot{
		Name: s.Name, Classification: string(s.Classification), ClinicalStatus: string(s.ClinicalStatus),
		AdministrativeStatus: string(s.AdministrativeStatus), MergedIntoID: s.MergedIntoID,
	}
	if s.CID11 != nil {
		result.CID11 = &ProblemCID11{Code: s.CID11.Code, System: s.CID11.System, Version: s.CID11.Version}
	}
	return result
}

func problemResponse(p problemdomain.Problem) ProblemResponse {
	return ProblemResponse{
		ProblemSnapshot: snapshotResponse(p.State), ID: p.ID, PatientID: p.PatientID,
		CreatedByAccountID: p.CreatedByAccountID, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version,
	}
}

func historyResponse(event problemdomain.HistoryEvent) ProblemHistoryEvent {
	result := ProblemHistoryEvent{
		ID: event.ID, ProblemID: event.ProblemID, PatientID: event.PatientID, Version: event.Version,
		Action: string(event.Action), ActorAccountID: event.ActorAccountID, OccurredAt: event.OccurredAt,
		AfterSnapshot: snapshotResponse(event.After), Reason: event.Reason,
		SourceProblemIDs: append([]uuid.UUID{}, event.SourceProblemIDs...),
	}
	if event.Before != nil {
		before := snapshotResponse(*event.Before)
		result.BeforeSnapshot = &before
	}
	return result
}
