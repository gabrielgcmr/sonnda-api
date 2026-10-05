// internal/features/patient/problem/repository.go
package problem

import (
	"context"
	"errors"

	problemdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/domain"
	"github.com/google/uuid"
)

var (
	ErrNotFound               = errors.New("patient problem not found")
	ErrVersionConflict        = errors.New("patient problem version conflict")
	ErrInconsistentAuditEvent = errors.New("patient problem and audit event are inconsistent")
)

// Repository persists each problem version and its audit event atomically.
type Repository interface {
	Create(ctx context.Context, problem problemdomain.Problem, event problemdomain.HistoryEvent) error
	Update(ctx context.Context, expectedVersion int64, problem problemdomain.Problem, event problemdomain.HistoryEvent) error
	Get(ctx context.Context, patientID, problemID uuid.UUID) (problemdomain.Problem, error)
	List(ctx context.Context, patientID uuid.UUID, filter ListFilter) ([]problemdomain.Problem, error)
	ListHistory(ctx context.Context, patientID, problemID uuid.UUID, page Pagination) ([]problemdomain.HistoryEvent, error)
}

type Pagination struct {
	Limit  int
	Offset int
}

type ListFilter struct {
	Pagination
	ClinicalStatus       string
	AdministrativeStatus string
}
