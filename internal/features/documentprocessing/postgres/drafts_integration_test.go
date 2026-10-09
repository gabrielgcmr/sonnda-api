// internal/features/documentprocessing/postgres/drafts_integration_test.go
//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	confirmation "github.com/gabrielgcmr/sonnda/internal/application/usecase/labdocumentconfirmation"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	processing "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	documents "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	labextract "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labextraction"
	labs "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/domain"
	labpostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/postgres"
	pginfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type confirmationAuthorizer struct {
	processing.Authorizer
	documents processing.DocumentRepository
}

func (a confirmationAuthorizer) AuthorizeDocument(ctx context.Context, _ *accountdomain.Account, id uuid.UUID, _ processing.Action) (*documents.ExamDocument, error) {
	return a.documents.FindByID(ctx, id)
}

func reviewTestDatabase(t *testing.T) (*pginfra.Client, *DraftRepository, uuid.UUID, uuid.UUID) {
	t.Helper()
	rawURL := os.Getenv("LABS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set LABS_TEST_DATABASE_URL to an isolated local Postgres")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("integration tests require a local Postgres URL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	schema := "processing_test_" + uuid.NewString()[:8]
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	params := u.Query()
	params.Set("search_path", schema)
	u.RawQuery = params.Encode()
	client, err := pginfra.NewClient(pginfra.Config{DatabaseURL: u.String(), MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if _, err := client.Pool().Exec(ctx, "CREATE TABLE accounts (id uuid PRIMARY KEY); CREATE TABLE patients (id uuid PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	schemaDir := filepath.Join("..", "..", "..", "infrastructure", "database", "postgres", "sqlc", "sql", "schema")
	for _, name := range []string{"exam.sql", "lab.sql"} {
		sql, err := os.ReadFile(filepath.Join(schemaDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Pool().Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	patientID, userID := uuid.New(), uuid.New()
	if _, err := client.Pool().Exec(ctx, "INSERT INTO accounts VALUES ($1); INSERT INTO patients VALUES ($2)", userID, patientID); err != nil {
		t.Fatal(err)
	}
	clinicalRepository := labpostgres.NewRepository(client)
	return client, NewDraftRepository(client, clinicalRepository), patientID, userID
}

func processingTestDocument(t *testing.T, client *pginfra.Client, patientID, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := client.Pool().Exec(context.Background(), `INSERT INTO exam_documents
		(id, patient_id, uploaded_by_user_id, storage_uri, original_filename, mime_type, status)
		VALUES ($1, $2, $3, 'test://exam.pdf', 'exam.pdf', 'application/pdf', 'uploaded')`, id, patientID, userID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func processingTestReport(patientID, userID uuid.UUID) *labs.LabReport {
	reportID, resultID := uuid.New(), uuid.New()
	value, unit := "99", "mg/dL"
	return &labs.LabReport{
		ID: reportID, PatientID: patientID, UploadedBy: userID,
		TestResults: []labs.LabPanel{{
			ID: resultID, LabReportID: reportID, TestName: "Glicose",
			Items: []labs.Observation{{
				ID: uuid.New(), LabPanelID: resultID, ParameterName: "Glicose", ResultValue: &value, ResultUnit: &unit,
			}},
		}},
	}
}

func createTestDraft(t *testing.T, repo *DraftRepository, patient, user uuid.UUID) *documents.ExamDocument {
	t.Helper()
	doc, err := documents.NewExamDocument(patient, user, "gs://test/exam.pdf", "exam.pdf", "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateDraft(context.Background(), doc, []byte(`{"version":1,"result":{"status":"partial"}}`)); err != nil {
		t.Fatal(err)
	}
	return doc
}
func TestDraftDoesNotCreateClinicalResults(t *testing.T) {
	client, repo, patient, user := reviewTestDatabase(t)
	doc := createTestDraft(t, repo, patient, user)
	var count int
	if err := client.Pool().QueryRow(context.Background(), "SELECT count(*) FROM lab_reports").Scan(&count); err != nil || count != 0 {
		t.Fatalf("clinical data before confirmation: %d %v", count, err)
	}
	data, err := repo.GetExtraction(context.Background(), doc.ID)
	if err != nil || !json.Valid(data) {
		t.Fatalf("snapshot not retained: %s %v", data, err)
	}
	loaded, err := repo.documents.FindByID(context.Background(), doc.ID)
	if err != nil || loaded.ReviewStatus == nil || *loaded.ReviewStatus != "pending" {
		t.Fatalf("bad draft: %+v %v", loaded, err)
	}
}
func TestConcurrentConfirmationIsAtomicAndIdempotent(t *testing.T) {
	client, repo, patient, user := reviewTestDatabase(t)
	doc := createTestDraft(t, repo, patient, user)
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := repo.Confirm(context.Background(), doc.ID, user, processingTestReport(patient, user), "same-report", nil)
			ids <- id
			failures <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first uuid.UUID
	for id := range ids {
		if first == uuid.Nil {
			first = id
		}
		if id != first {
			t.Fatal("confirmation duplicated")
		}
	}
	var reports, panels, observations int
	if err := client.Pool().QueryRow(context.Background(), "SELECT (SELECT count(*) FROM lab_reports),(SELECT count(*) FROM lab_panels),(SELECT count(*) FROM observations)").Scan(&reports, &panels, &observations); err != nil || reports != 1 || panels != 1 || observations != 1 {
		t.Fatalf("unexpected counts %d/%d/%d %v", reports, panels, observations, err)
	}
	loaded, err := repo.documents.FindByID(context.Background(), doc.ID)
	if err != nil || loaded.ConfirmedAt == nil || loaded.ConfirmedByUserID == nil || *loaded.ConfirmedByUserID != user || loaded.LabReportID == nil || *loaded.LabReportID != first {
		t.Fatalf("confirmation metadata: %+v %v", loaded, err)
	}
	if _, err := repo.BeginDelete(context.Background(), doc.ID); !errors.Is(err, documents.ErrReviewConflict) {
		t.Fatalf("confirmed document deletable: %v", err)
	}
}
func TestConfirmationFailureRollsBackClinicalData(t *testing.T) {
	client, repo, patient, user := reviewTestDatabase(t)
	doc := createTestDraft(t, repo, patient, user)
	// Foreign-key failure after inserting all clinical records must undo everything.
	_, err := repo.Confirm(context.Background(), doc.ID, uuid.New(), processingTestReport(patient, user), "rollback", nil)
	if err == nil {
		t.Fatal("expected confirmation failure")
	}
	var count int
	_ = client.Pool().QueryRow(context.Background(), "SELECT count(*) FROM lab_reports").Scan(&count)
	loaded, _ := repo.documents.FindByID(context.Background(), doc.ID)
	if count != 0 || loaded.ReviewStatus == nil || *loaded.ReviewStatus != "pending" {
		t.Fatal("partial transaction committed")
	}
}
func TestDuplicateExamAndDeleteRetry(t *testing.T) {
	_, repo, patient, user := reviewTestDatabase(t)
	one, two := createTestDraft(t, repo, patient, user), createTestDraft(t, repo, patient, user)
	if _, err := repo.Confirm(context.Background(), one.ID, user, processingTestReport(patient, user), "duplicate", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Confirm(context.Background(), two.ID, user, processingTestReport(patient, user), "duplicate", nil); !errors.Is(err, labs.ErrLabReportAlreadyExists) {
		t.Fatalf("duplicate accepted: %v", err)
	}
	for range 2 {
		if _, err := repo.BeginDelete(context.Background(), two.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.Confirm(context.Background(), two.ID, user, processingTestReport(patient, user), "another", nil); !errors.Is(err, documents.ErrReviewConflict) {
		t.Fatalf("deleting draft confirmed: %v", err)
	}
	for range 2 {
		if err := repo.FinishDelete(context.Background(), two.ID); err != nil {
			t.Fatal(err)
		}
	}
	if data, err := repo.GetExtraction(context.Background(), two.ID); err != nil || len(data) != 0 {
		t.Fatal("snapshot not deleted")
	}
}
func TestLegacyDocumentCannotBeConfirmedOrDiscarded(t *testing.T) {
	client, repo, patient, user := reviewTestDatabase(t)
	id := processingTestDocument(t, client, patient, user)
	if _, err := repo.BeginDelete(context.Background(), id); !errors.Is(err, documents.ErrReviewConflict) {
		t.Fatalf("legacy document deleted: %v", err)
	}
	if _, err := repo.Confirm(context.Background(), id, user, processingTestReport(patient, user), "legacy", nil); !errors.Is(err, documents.ErrReviewConflict) {
		t.Fatalf("legacy document confirmed: %v", err)
	}
}

func TestConfirmationUsesStoredSnapshotWithoutReextracting(t *testing.T) {
	client, repo, patient, user := reviewTestDatabase(t)
	value, unit, reference, date, raw := "< 5", "mg/dL", "70–99", "2026-09-01", "Original extracted text"
	result := &extraction.Result{Status: labextract.ExtractionStatusPartial, SummaryText: "Glicose: < 5 mg/dL", Report: labextract.ExtractedLabReport{
		RawText: &raw, Tests: []labextract.ExtractedTestResult{{TestName: "Glicose", CollectedAt: &date, Items: []labextract.ExtractedTestItem{{ParameterName: "Glicose", ResultValue: &value, ResultUnit: &unit, ReferenceText: &reference}}}},
	}}
	data, err := processing.EncodeExtractionSnapshot(result)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := documents.NewExamDocument(patient, user, "gs://test/file.pdf", "file.pdf", "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateDraft(context.Background(), doc, data); err != nil {
		t.Fatal(err)
	}
	authorizer := confirmationAuthorizer{documents: repo.documents}
	service := confirmation.New(authorizer, repo, labpostgres.NewRepository(client))
	account := &accountdomain.Account{ID: user, AccountType: accountdomain.AccountTypeBasicCare}
	saved, err := service.Confirm(context.Background(), account, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	item := saved.Panels[0].Observations[0]
	// pgx may return the same instant in the host's local timezone.
	if *item.ResultValue != value || *item.ResultUnit != unit || *item.ReferenceText != reference || saved.Panels[0].CollectedAt.UTC().Format("2006-01-02") != date || saved.ExamDocumentID == nil {
		t.Fatalf("confirmation changed reviewed values: %+v", saved)
	}
	again, err := service.Confirm(context.Background(), account, doc.ID)
	if err != nil || again.ID != saved.ID {
		t.Fatalf("retry changed report: %v", err)
	}
	var storedRaw string
	if err := client.Pool().QueryRow(context.Background(), "SELECT raw_text FROM lab_reports WHERE id=$1", saved.ID).Scan(&storedRaw); err != nil || storedRaw != raw {
		t.Fatalf("raw text lost: %v", err)
	}
}

func TestReviewMigrationPreservesLegacyAndProtectsSnapshots(t *testing.T) {
	client, repo, patient, user := reviewTestDatabase(t)
	legacy := processingTestDocument(t, client, patient, user)
	ctx := context.Background()
	// Recreate the pre-review shape, then apply the actual migration in this isolated schema.
	_, err := client.Pool().Exec(ctx, `ALTER TABLE accounts RENAME TO users;
 DROP TABLE exam_document_extractions;
 ALTER TABLE exam_documents DROP COLUMN review_status,DROP COLUMN lab_report_id,DROP COLUMN confirmed_by_user_id,DROP COLUMN confirmed_at;
 DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='anon') THEN CREATE ROLE anon; END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN CREATE ROLE authenticated; END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='service_role') THEN CREATE ROLE service_role; END IF;
 END $$;`)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "..", "..", "supabase", "migrations", "20260930211213_lab_document_review.sql")
	migration, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Pool().Exec(ctx, strings.ReplaceAll(string(migration), "public.", "")); err != nil {
		t.Fatal(err)
	}
	// Restore the account table name after validating the historical migration.
	if _, err = client.Pool().Exec(ctx, "ALTER TABLE users RENAME TO accounts"); err != nil {
		t.Fatal(err)
	}
	doc, err := repo.documents.FindByID(ctx, legacy)
	if err != nil || doc == nil || doc.ReviewStatus != nil {
		t.Fatalf("legacy changed: %+v %v", doc, err)
	}
	draft := createTestDraft(t, repo, patient, user)
	if _, err = client.Pool().Exec(ctx, "UPDATE exam_document_extractions SET snapshot='{}' WHERE document_id=$1", draft.ID); err == nil {
		t.Fatal("snapshot is mutable")
	}
	for _, table := range []string{"exam_documents", "exam_document_extractions", "exam_document_texts"} {
		var rls, allowed bool
		err = client.Pool().QueryRow(ctx, `SELECT relrowsecurity,has_table_privilege('authenticated',oid,'SELECT') FROM pg_class WHERE oid=$1::regclass`, table).Scan(&rls, &allowed)
		if err != nil || !rls || allowed {
			t.Fatalf("table exposed: %s, RLS=%v permission=%v error=%v", table, rls, allowed, err)
		}
	}
	report := processingTestReport(patient, user)
	if _, err = repo.Confirm(ctx, draft.ID, user, report, "migrated", nil); err != nil {
		t.Fatal(err)
	}
}
