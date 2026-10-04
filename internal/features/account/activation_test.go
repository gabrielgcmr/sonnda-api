// internal/features/account/activation_test.go
package account

import (
	"context"
	"errors"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type activationRepository struct {
	Repository
	user        *accountdomain.User
	findErr     error
	updateErr   error
	updateCalls int
}

func (r *activationRepository) FindByID(context.Context, uuid.UUID) (*accountdomain.User, error) {
	return r.user, r.findErr
}

func (r *activationRepository) ActivateProfessional(context.Context, uuid.UUID) (*accountdomain.User, error) {
	r.updateCalls++
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	r.user.AccountType = accountdomain.AccountTypeProfessional
	r.user.UpdatedAt = time.Now().UTC()
	return r.user, nil
}

type activationLimiter struct {
	failures  int
	checkErr  error
	recordErr error
}

func (l *activationLimiter) CanAttempt(_ context.Context, _ uuid.UUID, _ string, limit int, _ time.Duration) (bool, error) {
	return l.failures < limit, l.checkErr
}

func (l *activationLimiter) RecordFailure(context.Context, uuid.UUID, string, time.Duration) error {
	l.failures++
	return l.recordErr
}

func TestProfessionalActivationPersistsTypeAndIsIdempotent(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repository := &activationRepository{user: activationUser(accountdomain.AccountTypeBasicCare)}
	limiter := &activationLimiter{}
	service := NewProfessionalActivationService(repository, limiter, ProfessionalActivationOptions{
		PasswordHash: string(hash), MaxAttempts: 5, Window: 15 * time.Minute,
	})

	activated, err := service.Activate(t.Context(), repository.user.ID, "correct-password", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if activated.AccountType != accountdomain.AccountTypeProfessional || repository.updateCalls != 1 {
		t.Fatalf("activation was not persisted: user=%+v updates=%d", activated, repository.updateCalls)
	}

	activated, err = service.Activate(t.Context(), repository.user.ID, "wrong-password", "192.0.2.1")
	if err != nil || activated.AccountType != accountdomain.AccountTypeProfessional || repository.updateCalls != 1 {
		t.Fatalf("repeated activation must be idempotent: user=%+v updates=%d error=%v", activated, repository.updateCalls, err)
	}
}

func TestProfessionalActivationRejectsInvalidPasswordAndLimitsFailures(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repository := &activationRepository{user: activationUser(accountdomain.AccountTypeBasicCare)}
	limiter := &activationLimiter{}
	service := NewProfessionalActivationService(repository, limiter, ProfessionalActivationOptions{
		PasswordHash: string(hash), MaxAttempts: 5, Window: 15 * time.Minute,
	})

	for attempt := 0; attempt < 5; attempt++ {
		if _, err := service.Activate(t.Context(), repository.user.ID, "wrong-password", "192.0.2.1"); appErrorKind(err) != apperr.ACCESS_DENIED {
			t.Fatalf("attempt %d: error=%v", attempt+1, err)
		}
	}
	if _, err := service.Activate(t.Context(), repository.user.ID, "correct-password", "192.0.2.1"); appErrorKind(err) != apperr.RATE_LIMIT_EXCEEDED {
		t.Fatalf("expected rate limit after five failures: %v", err)
	}
	if repository.updateCalls != 0 {
		t.Fatal("rejected activation reached persistence")
	}
}

func TestProfessionalActivationFailsClosedWhenDependenciesOrConfigurationFail(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		hash    string
		limiter ActivationLimiter
	}{
		{name: "missing hash", limiter: &activationLimiter{}},
		{name: "invalid hash", hash: "not-bcrypt", limiter: &activationLimiter{}},
		{name: "missing limiter", hash: string(hash)},
		{name: "limiter failure", hash: string(hash), limiter: &activationLimiter{checkErr: errors.New("redis unavailable")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &activationRepository{user: activationUser(accountdomain.AccountTypeBasicCare)}
			service := NewProfessionalActivationService(repository, test.limiter, ProfessionalActivationOptions{
				PasswordHash: test.hash, MaxAttempts: 5, Window: 15 * time.Minute,
			})
			if _, err := service.Activate(t.Context(), repository.user.ID, "correct-password", "192.0.2.1"); appErrorKind(err) != apperr.INTERNAL_ERROR {
				t.Fatalf("expected internal error: %v", err)
			}
			if repository.updateCalls != 0 {
				t.Fatal("failed-closed activation reached persistence")
			}
		})
	}
}

func activationUser(accountType accountdomain.AccountType) *accountdomain.User {
	return &accountdomain.User{ID: uuid.New(), AccountType: accountType, UpdatedAt: time.Now().Add(-time.Hour)}
}

func appErrorKind(err error) apperr.ErrorKind {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Kind
	}
	return ""
}
