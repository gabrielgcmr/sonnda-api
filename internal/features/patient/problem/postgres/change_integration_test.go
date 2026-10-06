// internal/features/patient/problem/postgres/change_integration_test.go
package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	problemservice "github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// Both operations read the same version before either can attempt its update.
type concurrentReadRepository struct {
	problemservice.Repository
	arrived chan struct{}
	release chan struct{}
}

func (r *concurrentReadRepository) Get(ctx context.Context, patientID, problemID uuid.UUID) (problemdomain.Problem, error) {
	p, err := r.Repository.Get(ctx, patientID, problemID)
	r.arrived <- struct{}{}
	select {
	case <-r.release:
		return p, err
	case <-ctx.Done():
		return problemdomain.Problem{}, ctx.Err()
	}
}

type changeTestAuthorizer struct{ professionalID uuid.UUID }

func (a changeTestAuthorizer) Authorize(_ context.Context, actorID, patientID uuid.UUID, action authz.ProblemAction) error {
	kind := accountdomain.AccountTypeBasicCare
	if actorID == a.professionalID {
		kind = accountdomain.AccountTypeProfessional
	}
	return authz.RequireProblemAction(action, patientID, authz.PatientContext{
		AccountID: actorID, PatientID: patientID, AccountType: kind, HasAccess: true,
	})
}

func TestServiceConcurrentChangesPreserveClinicalRulesAndAudit(t *testing.T) {
	for _, scenario := range []string{"resolve versus classify", "simultaneous resolutions"} {
		t.Run(scenario, func(t *testing.T) {
			client, repository, patientID, professionalID := newProblemTestDatabase(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			careID := uuid.New()
			if _, err := client.Pool().Exec(ctx, "INSERT INTO users (id) VALUES ($1)", careID); err != nil {
				t.Fatal(err)
			}
			p, created := testProblem(t, patientID, professionalID, "Problema agudo")
			if err := repository.Create(ctx, p, created); err != nil {
				t.Fatal(err)
			}
			barrier := &concurrentReadRepository{Repository: repository, arrived: make(chan struct{}, 2), release: make(chan struct{})}
			service := problemservice.New(barrier, changeTestAuthorizer{professionalID})
			results := make(chan error, 2)
			go func() {
				_, err := service.Resolve(ctx, careID, patientID, p.ID, 1)
				results <- err
			}()
			go func() {
				var err error
				if scenario == "resolve versus classify" {
					_, err = service.Classify(ctx, professionalID, patientID, p.ID, problemservice.ClassifyInput{Version: 1, Classification: problemdomain.ClassificationChronic})
				} else {
					_, err = service.Resolve(ctx, professionalID, patientID, p.ID, 1)
				}
				results <- err
			}()
			for range 2 {
				select {
				case <-barrier.arrived:
				case <-ctx.Done():
					close(barrier.release)
					t.Fatal("concurrent reads did not complete")
				}
			}
			close(barrier.release)
			successes, conflicts := 0, 0
			for range 2 {
				select {
				case err := <-results:
					var appError *apperr.AppError
					if err == nil {
						successes++
					} else if errors.As(err, &appError) && appError.Kind == apperr.RESOURCE_CONFLICT {
						conflicts++
					} else {
						t.Fatalf("unexpected concurrent failure: %v", err)
					}
				case <-ctx.Done():
					t.Fatal("concurrent updates did not complete")
				}
			}
			if successes != 1 || conflicts != 1 {
				t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
			}
			current, err := repository.Get(ctx, patientID, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Version != 2 || current.Validate() != nil {
				t.Fatalf("invalid final problem: %+v", current)
			}
			events, err := repository.ListHistory(ctx, patientID, p.ID, problemservice.Pagination{Limit: 10})
			if err != nil || len(events) != 2 || events[0].Version != 2 || !snapshotsEqual(events[0].After, current.State) || !snapshotsEqual(*events[0].Before, p.State) {
				t.Fatalf("invalid concurrent audit: %+v, %v", events, err)
			}
			if events[0].Action == problemdomain.ActionClassified && events[0].ActorAccountID != professionalID {
				t.Fatal("classification did not preserve professional authorship")
			}
			if events[0].Action == problemdomain.ActionResolved && events[0].ActorAccountID != professionalID && events[0].ActorAccountID != careID {
				t.Fatal("resolution did not preserve account authorship")
			}
			// A repeat of either submitted request cannot create another version.
			plainService := problemservice.New(repository, changeTestAuthorizer{professionalID})
			_, err = plainService.Resolve(ctx, careID, patientID, p.ID, 1)
			var appError *apperr.AppError
			if !errors.As(err, &appError) || appError.Kind != apperr.RESOURCE_CONFLICT {
				t.Fatalf("repeat should conflict: %v", err)
			}
			if scenario == "resolve versus classify" {
				if current.State.Classification == problemdomain.ClassificationChronic {
					_, err = plainService.Resolve(ctx, careID, patientID, p.ID, 2)
				} else {
					_, err = plainService.Classify(ctx, professionalID, patientID, p.ID, problemservice.ClassifyInput{Version: 2, Classification: problemdomain.ClassificationChronic})
				}
				if !errors.As(err, &appError) || appError.Kind != apperr.DOMAIN_RULE_VIOLATION {
					t.Fatalf("retry with current version should reject chronic resolved: %v", err)
				}
			}
			events, err = repository.ListHistory(ctx, patientID, p.ID, problemservice.Pagination{Limit: 10})
			if err != nil || len(events) != 2 {
				t.Fatalf("failed requests appended audit: %+v %v", events, err)
			}
		})
	}
}
