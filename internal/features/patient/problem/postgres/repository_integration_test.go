// internal/features/patient/problem/postgres/repository_integration_test.go
package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	problemrepository "github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	pginfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func newProblemTestDatabase(t *testing.T) (*pginfra.Client, *Repository, uuid.UUID, uuid.UUID) {
	t.Helper()
	rawURL := os.Getenv("PROBLEMS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("set PROBLEMS_TEST_DATABASE_URL to an isolated local Postgres")
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

	schema := "problem_test_" + uuid.NewString()[:8]
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

	if _, err := client.Pool().Exec(ctx, "CREATE TABLE users (id uuid PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Pool().Exec(ctx, "CREATE TABLE patients (id uuid PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	schemaPath := filepath.Join("..", "..", "..", "..", "infrastructure", "database", "postgres", "sqlc", "sql", "schema", "problem.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Pool().Exec(ctx, string(schemaSQL)); err != nil {
		t.Fatal(err)
	}

	patientID, actorID := uuid.New(), uuid.New()
	if _, err := client.Pool().Exec(ctx, "INSERT INTO users (id) VALUES ($1)", actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Pool().Exec(ctx, "INSERT INTO patients (id) VALUES ($1)", patientID); err != nil {
		t.Fatal(err)
	}
	return client, NewRepository(client), patientID, actorID
}

func testProblem(t *testing.T, patientID, actorID uuid.UUID, name string) (problemdomain.Problem, problemdomain.HistoryEvent) {
	t.Helper()
	problem, event, err := problemdomain.NewProblem(problemdomain.NewProblemParams{
		PatientID: patientID, ActorAccountID: actorID, Name: name,
		Classification: problemdomain.ClassificationAcute, OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return problem, event
}

func TestRepositoryCreateAndUpdateAreVersionedAndAtomic(t *testing.T) {
	client, repository, patientID, actorID := newProblemTestDatabase(t)
	ctx := context.Background()
	problem, created := testProblem(t, patientID, actorID, "Problema agudo")
	if err := repository.Create(ctx, problem, created); err != nil {
		t.Fatal(err)
	}

	after := problem.State
	after.ClinicalStatus = problemdomain.ClinicalStatusResolved
	updated, changed, err := problem.Change(problemdomain.ChangeParams{
		Action: problemdomain.ActionResolved, After: after,
		ActorAccountID: actorID, OccurredAt: problem.UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Update(ctx, 1, updated, changed); err != nil {
		t.Fatal(err)
	}

	var version, historyCount int64
	var status string
	if err := client.Pool().QueryRow(ctx, "SELECT version, clinical_status FROM patient_problems WHERE id=$1", problem.ID).Scan(&version, &status); err != nil {
		t.Fatal(err)
	}
	if err := client.Pool().QueryRow(ctx, "SELECT count(*) FROM patient_problem_history WHERE problem_id=$1", problem.ID).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if version != 2 || status != "resolved" || historyCount != 2 {
		t.Fatalf("version=%d status=%s history=%d", version, status, historyCount)
	}

	if err := repository.Update(ctx, 1, updated, changed); !errors.Is(err, problemrepository.ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestRepositoryRollsBackWhenHistoryInsertFails(t *testing.T) {
	client, repository, patientID, actorID := newProblemTestDatabase(t)
	ctx := context.Background()
	problem, created := testProblem(t, patientID, actorID, "Primeiro")
	if err := repository.Create(ctx, problem, created); err != nil {
		t.Fatal(err)
	}

	second, secondEvent := testProblem(t, patientID, actorID, "Segundo")
	secondEvent.ID = created.ID
	if err := repository.Create(ctx, second, secondEvent); !errors.Is(err, persistence.ErrPersistenceFailure) {
		t.Fatalf("expected persistence failure, got %v", err)
	}
	var count int
	if err := client.Pool().QueryRow(ctx, "SELECT count(*) FROM patient_problems WHERE id=$1", second.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("problem insert was not rolled back: count=%d", count)
	}
}

func TestRepositoryRollsBackUpdateWhenHistoryInsertFails(t *testing.T) {
	client, repository, patientID, actorID := newProblemTestDatabase(t)
	ctx := context.Background()
	problem, created := testProblem(t, patientID, actorID, "Problema")
	if err := repository.Create(ctx, problem, created); err != nil {
		t.Fatal(err)
	}
	after := problem.State
	after.Name = "Nome alterado"
	updated, changed, err := problem.Change(problemdomain.ChangeParams{
		Action: problemdomain.ActionEdited, After: after,
		ActorAccountID: actorID, OccurredAt: problem.UpdatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	changed.ID = created.ID
	if err := repository.Update(ctx, 1, updated, changed); !errors.Is(err, persistence.ErrPersistenceFailure) {
		t.Fatalf("expected persistence failure, got %v", err)
	}
	var version int64
	var name string
	if err := client.Pool().QueryRow(ctx, "SELECT version, name FROM patient_problems WHERE id=$1", problem.ID).Scan(&version, &name); err != nil {
		t.Fatal(err)
	}
	if version != 1 || name != "Problema" {
		t.Fatalf("update was not rolled back: version=%d name=%s", version, name)
	}
}

func TestDatabaseRejectsChronicResolved(t *testing.T) {
	client, _, patientID, actorID := newProblemTestDatabase(t)
	_, err := client.Pool().Exec(context.Background(), `
		INSERT INTO patient_problems (
			id, patient_id, name, classification, clinical_status,
			administrative_status, created_by_account_id, created_at, updated_at, version
		) VALUES ($1,$2,'Crônico','chronic','resolved','valid',$3,now(),now(),1)`,
		uuid.New(), patientID, actorID)
	if err == nil {
		t.Fatal("database accepted chronic + resolved")
	}
}

func TestDatabaseRejectsIncompleteCID11(t *testing.T) {
	client, _, patientID, actorID := newProblemTestDatabase(t)
	_, err := client.Pool().Exec(context.Background(), `
		INSERT INTO patient_problems (
			id, patient_id, name, cid11_code, classification, clinical_status,
			administrative_status, created_by_account_id, created_at, updated_at, version
		) VALUES ($1,$2,'CID incompleto','CA23','acute','active','valid',$3,now(),now(),1)`,
		uuid.New(), patientID, actorID)
	if err == nil {
		t.Fatal("database accepted an incomplete CID-11 coding")
	}
}
