// internal/features/documentprocessing/postgres/draft.go
package postgres

import (
	"context"
	"errors"
	"time"

	documents "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/domain"
	labs "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/domain"
	pginfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	examsqlc "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/sqlc/generated/exam"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type DraftRepository struct {
	client    *pginfra.Client
	documents *ExamsRepository
	labs      labReportWriter
}

type labReportWriter interface {
	CreateInTx(context.Context, pgx.Tx, *labs.LabReport) error
}

func NewDraftRepository(client *pginfra.Client, labs labReportWriter) *DraftRepository {
	return &DraftRepository{client: client, documents: &ExamsRepository{client: client, queries: examsqlc.New(client.Pool())}, labs: labs}
}

func rollback(ctx context.Context, tx pgx.Tx) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(cleanup)
}

func (r *DraftRepository) CreateDraft(ctx context.Context, doc *documents.ExamDocument, snapshot []byte) error {
	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	_, err = tx.Exec(ctx, `INSERT INTO exam_documents
 (id, patient_id, uploaded_by_user_id, storage_uri, original_filename, mime_type, status, exam_type, review_status, extraction_method, created_at, updated_at)
 VALUES ($1,$2,$3,$4,$5,'application/pdf','processed','laboratory','pending','pdf_text_raw',$6,$7)`,
		doc.ID, doc.PatientID, doc.UploadedByUserID, doc.StorageURI, doc.OriginalFilename, doc.CreatedAt, doc.UpdatedAt)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO exam_document_extractions(document_id,snapshot) VALUES ($1,$2)`, doc.ID, string(snapshot)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *DraftRepository) GetExtraction(ctx context.Context, id uuid.UUID) ([]byte, error) {
	data, err := r.documents.queries.GetExamDocumentExtraction(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return data, err
}

func (r *DraftRepository) BeginDelete(ctx context.Context, id uuid.UUID) (*documents.ExamDocument, error) {
	var uri string
	err := r.client.Pool().QueryRow(ctx, `UPDATE exam_documents SET review_status='deleting', updated_at=now()
 WHERE id=$1 AND review_status IN ('pending','deleting') RETURNING storage_uri`, id).Scan(&uri)
	if errors.Is(err, pgx.ErrNoRows) {
		document, lookupErr := r.documents.FindByID(ctx, id)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if document == nil {
			return nil, nil
		}
		return nil, documents.ErrReviewConflict
	}
	if err != nil {
		return nil, err
	}
	return &documents.ExamDocument{ID: id, StorageURI: uri}, nil
}

func (r *DraftRepository) FinishDelete(ctx context.Context, id uuid.UUID) error {
	_, err := r.client.Pool().Exec(ctx, `DELETE FROM exam_documents WHERE id=$1 AND review_status='deleting'`, id)
	return err
}

func (r *DraftRepository) Confirm(ctx context.Context, id, userID uuid.UUID, report *labs.LabReport, fingerprint string, rawText *string) (uuid.UUID, error) {
	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer rollback(ctx, tx)
	var state pgtype.Text
	var linked pgtype.UUID
	var patientID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT review_status,lab_report_id,patient_id FROM exam_documents WHERE id=$1 FOR UPDATE`, id).Scan(&state, &linked, &patientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, documents.ErrReviewConflict
	}
	if err != nil {
		return uuid.Nil, err
	}
	if state.String == "confirmed" && linked.Valid {
		return uuid.UUID(linked.Bytes), nil
	}
	if state.String != "pending" || report == nil || report.PatientID != patientID {
		return uuid.Nil, documents.ErrReviewConflict
	}

	reportID := report.ID
	var existingID uuid.UUID
	var existingDocument pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT id,exam_document_id FROM lab_reports WHERE patient_id=$1 AND fingerprint=$2 FOR UPDATE`, patientID, fingerprint).Scan(&existingID, &existingDocument)
	switch {
	case err == nil:
		if existingDocument.Valid && uuid.UUID(existingDocument.Bytes) != id {
			return uuid.Nil, labs.ErrLabReportAlreadyExists
		}
		reportID = existingID
	case errors.Is(err, pgx.ErrNoRows):
		if err = r.labs.CreateInTx(ctx, tx, report); err != nil {
			return uuid.Nil, reviewWriteError(err)
		}
	default:
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE lab_reports SET exam_document_id=$2, fingerprint=$3,raw_text=$4,updated_at=now() WHERE id=$1`, reportID, id, fingerprint, rawText)
	if err != nil {
		return uuid.Nil, reviewWriteError(err)
	}
	_, err = tx.Exec(ctx, `UPDATE exam_documents SET review_status='confirmed',lab_report_id=$2,confirmed_by_user_id=$3,confirmed_at=now(),updated_at=now() WHERE id=$1`, id, reportID, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return reportID, nil
}

func reviewWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && (pgErr.ConstraintName == "idx_lab_reports_fingerprint" || pgErr.ConstraintName == "idx_lab_reports_exam_document") {
		return errors.Join(labs.ErrLabReportAlreadyExists, err)
	}
	return err
}
