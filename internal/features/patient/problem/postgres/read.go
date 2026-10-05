// internal/features/patient/problem/postgres/read.go
package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	problemsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/problem"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) Get(ctx context.Context, patientID, problemID uuid.UUID) (problemdomain.Problem, error) {
	row, err := r.queries.GetPatientProblem(ctx, problemsqlc.GetPatientProblemParams{PatientID: patientID, ID: problemID})
	if errors.Is(err, pgx.ErrNoRows) {
		return problemdomain.Problem{}, problem.ErrNotFound
	}
	if err != nil {
		return problemdomain.Problem{}, persistenceError("get patient problem", err)
	}
	return problemFromRow(row)
}

func (r *Repository) List(ctx context.Context, patientID uuid.UUID, filter problem.ListFilter) ([]problemdomain.Problem, error) {
	rows, err := r.queries.ListPatientProblems(ctx, problemsqlc.ListPatientProblemsParams{
		PatientID: patientID, ClinicalFilter: filter.ClinicalStatus, AdministrativeFilter: filter.AdministrativeStatus,
		PageLimit: int32(filter.Limit), PageOffset: int32(filter.Offset),
	})
	if err != nil {
		return nil, persistenceError("list patient problems", err)
	}
	items := make([]problemdomain.Problem, len(rows))
	for i, row := range rows {
		items[i], err = problemFromRow(row)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r *Repository) ListHistory(ctx context.Context, patientID, problemID uuid.UUID, page problem.Pagination) ([]problemdomain.HistoryEvent, error) {
	rows, err := r.queries.ListPatientProblemHistory(ctx, problemsqlc.ListPatientProblemHistoryParams{
		PatientID: patientID, ProblemID: problemID, PageLimit: int32(page.Limit), PageOffset: int32(page.Offset),
	})
	if err != nil {
		return nil, persistenceError("list patient problem history", err)
	}
	items := make([]problemdomain.HistoryEvent, len(rows))
	for i, row := range rows {
		items[i], err = historyFromRow(row)
		if err != nil {
			return nil, persistenceError("decode patient problem history", err)
		}
	}
	return items, nil
}

func problemFromRow(row problemsqlc.PatientProblem) (problemdomain.Problem, error) {
	p := problemdomain.Problem{
		ID: row.ID, PatientID: row.PatientID, CreatedByAccountID: row.CreatedByAccountID,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), Version: row.Version,
		State: problemdomain.Snapshot{
			Name: row.Name, Classification: problemdomain.Classification(row.Classification),
			ClinicalStatus:       problemdomain.ClinicalStatus(row.ClinicalStatus),
			AdministrativeStatus: problemdomain.AdministrativeStatus(row.AdministrativeStatus),
		},
	}
	if row.Cid11Code.Valid {
		p.State.CID11 = &problemdomain.CID11{Code: row.Cid11Code.String, System: row.Cid11System.String, Version: row.Cid11Version.String}
	}
	if row.MergedIntoID.Valid {
		id := uuid.UUID(row.MergedIntoID.Bytes)
		p.State.MergedIntoID = &id
	}
	if err := p.Validate(); err != nil {
		return problemdomain.Problem{}, persistenceError("decode patient problem", err)
	}
	return p, nil
}

func historyFromRow(row problemsqlc.PatientProblemHistory) (problemdomain.HistoryEvent, error) {
	event := problemdomain.HistoryEvent{
		ID: row.ID, ProblemID: row.ProblemID, PatientID: row.PatientID, Version: row.Version,
		Action: problemdomain.Action(row.Action), ActorAccountID: row.ActorAccountID,
		OccurredAt: row.OccurredAt.Time.UTC(), Reason: row.Reason.String, SourceProblemIDs: row.SourceProblemIds,
	}
	var err error
	event.After, err = decodeSnapshot(row.AfterSnapshot, row.ProblemID)
	if err != nil {
		return event, err
	}
	if len(row.BeforeSnapshot) > 0 {
		before, err := decodeSnapshot(row.BeforeSnapshot, row.ProblemID)
		if err != nil {
			return event, err
		}
		event.Before = &before
	}
	return event, nil
}

func decodeSnapshot(data []byte, problemID uuid.UUID) (problemdomain.Snapshot, error) {
	var stored auditSnapshot
	if err := json.Unmarshal(data, &stored); err != nil {
		return problemdomain.Snapshot{}, err
	}
	snapshot := problemdomain.Snapshot{
		Name: stored.Name, Classification: stored.Classification, ClinicalStatus: stored.ClinicalStatus,
		AdministrativeStatus: stored.AdministrativeStatus, MergedIntoID: stored.MergedIntoID,
	}
	if stored.CID11 != nil {
		snapshot.CID11 = &problemdomain.CID11{Code: stored.CID11.Code, System: stored.CID11.System, Version: stored.CID11.Version}
	}
	return snapshot, snapshot.Validate(problemID)
}
