// internal/features/patient/problem/repository.go
package problem

import (
	"context"
	"errors"

	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
)

var (
	ErrVersionConflict        = errors.New("patient problem version conflict")
	ErrInconsistentAuditEvent = errors.New("patient problem and audit event are inconsistent")
)

// Repository persists each problem version and its audit event atomically.
type Repository interface {
	Create(ctx context.Context, problem problemdomain.Problem, event problemdomain.HistoryEvent) error
	Update(ctx context.Context, expectedVersion int64, problem problemdomain.Problem, event problemdomain.HistoryEvent) error
}
