// internal/features/patient/exam/laboratory/postgres/repository_integration_test.go
//go:build integration

package postgres

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	labs "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/domain"
	pginfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func newRepositoryTestDatabase(t *testing.T) (*pginfra.Client, *Repository, uuid.UUID, uuid.UUID) {
	t.Helper()
	rawURL := os.Getenv("LABS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set LABS_TEST_DATABASE_URL to an isolated local Postgres")
	}

	databaseURL, err := url.Parse(rawURL)
	if err != nil || (databaseURL.Hostname() != "127.0.0.1" && databaseURL.Hostname() != "localhost") {
		t.Fatal("integration tests require a local Postgres URL")
	}

	ctx := context.Background()
	admin, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })

	schema := "laboratory_test_" + uuid.NewString()[:8]
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	})

	params := databaseURL.Query()
	params.Set("search_path", schema)
	databaseURL.RawQuery = params.Encode()
	client, err := pginfra.NewClient(pginfra.Config{DatabaseURL: databaseURL.String(), MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	if _, err := client.Pool().Exec(ctx, "CREATE TABLE accounts (id uuid PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Pool().Exec(ctx, "CREATE TABLE patients (id uuid PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	schemaDir := filepath.Join("..", "..", "..", "..", "..", "infrastructure", "database", "postgres", "sqlc", "sql", "schema")
	for _, name := range []string{"exam.sql", "lab.sql"} {
		schemaSQL, err := os.ReadFile(filepath.Join(schemaDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Pool().Exec(ctx, string(schemaSQL)); err != nil {
			t.Fatal(err)
		}
	}

	patientID, userID := uuid.New(), uuid.New()
	if _, err := client.Pool().Exec(ctx, "INSERT INTO accounts (id) VALUES ($1)", userID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Pool().Exec(ctx, "INSERT INTO patients (id) VALUES ($1)", patientID); err != nil {
		t.Fatal(err)
	}

	return client, NewRepository(client), patientID, userID
}

func repositoryTestReport(patientID, userID uuid.UUID, reportDate time.Time, value string) *labs.LabReport {
	name, labName, testName := "Paciente de teste", "Laboratório de teste", "Hemograma"
	unit, reference := "g/dL", "12-16"
	reportID := uuid.New()
	panelID := uuid.New()
	return &labs.LabReport{
		ID:          reportID,
		PatientID:   patientID,
		PatientName: &name,
		LabName:     &labName,
		ReportDate:  &reportDate,
		UploadedBy:  userID,
		TestResults: []labs.LabPanel{{
			ID:          panelID,
			LabReportID: reportID,
			TestName:    testName,
			CollectedAt: &reportDate,
			Items: []labs.Observation{{
				ID:            uuid.New(),
				LabPanelID:    panelID,
				ParameterName: "Hemoglobina",
				ResultValue:   &value,
				ResultUnit:    &unit,
				ReferenceText: &reference,
			}},
		}},
	}
}

func assertRepositoryCounts(t *testing.T, client *pginfra.Client, reports, panels, observations int) {
	t.Helper()
	var actualReports, actualPanels, actualObservations int
	if err := client.Pool().QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM lab_reports),
			(SELECT count(*) FROM lab_panels),
			(SELECT count(*) FROM observations)
	`).Scan(&actualReports, &actualPanels, &actualObservations); err != nil {
		t.Fatal(err)
	}
	if actualReports != reports || actualPanels != panels || actualObservations != observations {
		t.Fatalf("expected counts %d/%d/%d, got %d/%d/%d", reports, panels, observations, actualReports, actualPanels, actualObservations)
	}
}

func TestRepositoryCreateAndFindByIDReconstructAggregate(t *testing.T) {
	client, repo, patientID, userID := newRepositoryTestDatabase(t)
	report := repositoryTestReport(patientID, userID, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "13.4")

	if err := repo.Create(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	assertRepositoryCounts(t, client, 1, 1, 1)

	saved, err := repo.FindByID(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || saved.ID != report.ID || saved.PatientID != patientID || saved.UploadedBy != userID {
		t.Fatalf("unexpected report: %+v", saved)
	}
	if saved.PatientName == nil || *saved.PatientName != "Paciente de teste" || saved.LabName == nil || *saved.LabName != "Laboratório de teste" {
		t.Fatalf("report metadata was not reconstructed: %+v", saved)
	}
	if len(saved.TestResults) != 1 || saved.TestResults[0].ID != report.TestResults[0].ID {
		t.Fatalf("expected one reconstructed panel, got %+v", saved.TestResults)
	}
	observations := saved.TestResults[0].Items
	if len(observations) != 1 || observations[0].ID != report.TestResults[0].Items[0].ID {
		t.Fatalf("expected one reconstructed observation, got %+v", observations)
	}
	if observations[0].ResultValue == nil || *observations[0].ResultValue != "13.4" || observations[0].ReferenceText == nil || *observations[0].ReferenceText != "12-16" {
		t.Fatalf("observation values were not reconstructed: %+v", observations[0])
	}
}

func TestRepositoryCreateRollsBackWhenPanelOrObservationInsertFails(t *testing.T) {
	for _, failure := range []string{"panel", "observation"} {
		t.Run(failure, func(t *testing.T) {
			client, repo, patientID, userID := newRepositoryTestDatabase(t)
			report := repositoryTestReport(patientID, userID, time.Now().UTC(), "13.4")
			switch failure {
			case "panel":
				duplicate := report.TestResults[0]
				report.TestResults = append(report.TestResults, duplicate)
			case "observation":
				duplicate := report.TestResults[0].Items[0]
				report.TestResults[0].Items = append(report.TestResults[0].Items, duplicate)
			}

			if err := repo.Create(context.Background(), report); err == nil {
				t.Fatal("expected insert failure")
			}
			assertRepositoryCounts(t, client, 0, 0, 0)
		})
	}
}

func TestRepositoryCreateInTxLeavesCommitToCaller(t *testing.T) {
	client, repo, patientID, userID := newRepositoryTestDatabase(t)
	report := repositoryTestReport(patientID, userID, time.Now().UTC(), "13.4")

	tx, err := client.BeginTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateInTx(context.Background(), tx, report); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertRepositoryCounts(t, client, 0, 0, 0)
}

func TestRepositoryFindByIDReturnsNilWhenMissing(t *testing.T) {
	_, repo, _, _ := newRepositoryTestDatabase(t)

	report, err := repo.FindByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if report != nil {
		t.Fatalf("expected nil report, got %+v", report)
	}
}

func TestRepositoryListLabsAndObservationTimeline(t *testing.T) {
	_, repo, patientID, userID := newRepositoryTestDatabase(t)
	older := repositoryTestReport(patientID, userID, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "12.8")
	newer := repositoryTestReport(patientID, userID, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "13.4")
	for _, report := range []*labs.LabReport{older, newer} {
		if err := repo.Create(context.Background(), report); err != nil {
			t.Fatal(err)
		}
	}

	reports, err := repo.ListLabs(context.Background(), patientID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].ID != newer.ID || len(reports[0].TestResults) != 1 || len(reports[0].TestResults[0].Items) != 1 {
		t.Fatalf("unexpected paged report list: %+v", reports)
	}

	timeline, err := repo.ListObservationTimelineByPatientAndParameter(context.Background(), patientID, "Hemoglobina", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 2 || timeline[0].ReportID != newer.ID || timeline[1].ReportID != older.ID {
		t.Fatalf("unexpected observation timeline: %+v", timeline)
	}
	if timeline[0].LabPanelID != newer.TestResults[0].ID || timeline[0].ObservationID != newer.TestResults[0].Items[0].ID {
		t.Fatalf("timeline lost panel/observation identifiers: %+v", timeline[0])
	}
}

func TestRepositoryDeleteCascadesPanelsAndObservations(t *testing.T) {
	client, repo, patientID, userID := newRepositoryTestDatabase(t)
	report := repositoryTestReport(patientID, userID, time.Now().UTC(), "13.4")
	if err := repo.Create(context.Background(), report); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(context.Background(), report.ID); err != nil {
		t.Fatal(err)
	}
	assertRepositoryCounts(t, client, 0, 0, 0)
}
