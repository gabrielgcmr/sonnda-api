// internal/features/patient/problem/postgres/mapper.go
package postgres

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	problemsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/problem"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type auditCID11 struct {
	Code    string `json:"code"`
	System  string `json:"system"`
	Version string `json:"version"`
}

type auditSnapshot struct {
	Name                 string                             `json:"name"`
	CID11                *auditCID11                        `json:"cid11,omitempty"`
	Classification       problemdomain.Classification       `json:"classification"`
	ClinicalStatus       problemdomain.ClinicalStatus       `json:"clinical_status"`
	AdministrativeStatus problemdomain.AdministrativeStatus `json:"administrative_status"`
	MergedIntoID         *uuid.UUID                         `json:"merged_into_id,omitempty"`
}

func problemParams(problem problemdomain.Problem) problemsqlc.CreatePatientProblemParams {
	cidCode, cidSystem, cidVersion := cid11Params(problem.State.CID11)
	return problemsqlc.CreatePatientProblemParams{
		ID: problem.ID, PatientID: problem.PatientID, Name: problem.State.Name,
		Cid11Code: cidCode, Cid11System: cidSystem, Cid11Version: cidVersion,
		Classification:       string(problem.State.Classification),
		ClinicalStatus:       string(problem.State.ClinicalStatus),
		AdministrativeStatus: string(problem.State.AdministrativeStatus),
		MergedIntoID:         nullableUUID(problem.State.MergedIntoID),
		CreatedByAccountID:   problem.CreatedByAccountID,
		CreatedAt:            timestamptz(problem.CreatedAt), UpdatedAt: timestamptz(problem.UpdatedAt),
		Version: problem.Version,
	}
}

func updateParams(expectedVersion int64, problem problemdomain.Problem) problemsqlc.UpdatePatientProblemVersionParams {
	created := problemParams(problem)
	return problemsqlc.UpdatePatientProblemVersionParams{
		Name: created.Name, Cid11Code: created.Cid11Code,
		Cid11System: created.Cid11System, Cid11Version: created.Cid11Version,
		Classification: created.Classification, ClinicalStatus: created.ClinicalStatus,
		AdministrativeStatus: created.AdministrativeStatus, MergedIntoID: created.MergedIntoID,
		UpdatedAt: created.UpdatedAt, Version: created.Version,
		ID: created.ID, PatientID: created.PatientID, ExpectedVersion: expectedVersion,
	}
}

func historyParams(event problemdomain.HistoryEvent) (problemsqlc.CreatePatientProblemHistoryParams, error) {
	after, err := json.Marshal(toAuditSnapshot(event.After))
	if err != nil {
		return problemsqlc.CreatePatientProblemHistoryParams{}, err
	}
	var before pgtype.Text
	if event.Before != nil {
		encodedBefore, marshalErr := json.Marshal(toAuditSnapshot(*event.Before))
		err = marshalErr
		if err != nil {
			return problemsqlc.CreatePatientProblemHistoryParams{}, err
		}
		before = pgtype.Text{String: string(encodedBefore), Valid: true}
	}
	return problemsqlc.CreatePatientProblemHistoryParams{
		ID: event.ID, ProblemID: event.ProblemID, PatientID: event.PatientID,
		Version: event.Version, Action: string(event.Action), ActorAccountID: event.ActorAccountID,
		OccurredAt: timestamptz(event.OccurredAt), BeforeSnapshot: before, AfterSnapshot: string(after),
		Reason: nullableText(event.Reason), SourceProblemIds: uuidArrayText(event.SourceProblemIDs),
	}, nil
}

func snapshotsEqual(left, right problemdomain.Snapshot) bool {
	leftJSON, leftErr := json.Marshal(toAuditSnapshot(left))
	rightJSON, rightErr := json.Marshal(toAuditSnapshot(right))
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func toAuditSnapshot(snapshot problemdomain.Snapshot) auditSnapshot {
	result := auditSnapshot{
		Name: snapshot.Name, Classification: snapshot.Classification,
		ClinicalStatus:       snapshot.ClinicalStatus,
		AdministrativeStatus: snapshot.AdministrativeStatus,
		MergedIntoID:         snapshot.MergedIntoID,
	}
	if snapshot.CID11 != nil {
		result.CID11 = &auditCID11{Code: snapshot.CID11.Code, System: snapshot.CID11.System, Version: snapshot.CID11.Version}
	}
	return result
}

func cid11Params(cid *problemdomain.CID11) (pgtype.Text, pgtype.Text, pgtype.Text) {
	if cid == nil {
		return pgtype.Text{}, pgtype.Text{}, pgtype.Text{}
	}
	return pgtype.Text{String: cid.Code, Valid: true},
		pgtype.Text{String: cid.System, Valid: true},
		pgtype.Text{String: cid.Version, Valid: true}
}

func nullableText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func nullableUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *value, Valid: true}
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func uuidArrayText(values []uuid.UUID) string {
	items := make([]string, len(values))
	for index, value := range values {
		items[index] = value.String()
	}
	return "{" + strings.Join(items, ",") + "}"
}
