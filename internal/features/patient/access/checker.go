// internal/features/patient/access/checker.go
package patientaccess

import (
	"context"
	"errors"
	"fmt"

	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// PatientLookup is the patient data required to decide whether an account has access.
type PatientLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*profiledomain.Patient, error)
}

// Checker verifies whether an account is linked to a patient.
type Checker interface {
	RequireAccess(ctx context.Context, accountID, patientID uuid.UUID) error
}

// Resolver verifies access and returns the patient loaded during authorization.
// Callers that need the profile should use this interface to avoid fetching it again.
type Resolver interface {
	Checker
	ResolveAccessiblePatient(ctx context.Context, accountID, patientID uuid.UUID) (*profiledomain.Patient, error)
}

type checker struct {
	patientRepo PatientLookup
	accessRepo  Repository
}

func NewChecker(patientRepo PatientLookup, accessRepo Repository) Resolver {
	return &checker{patientRepo: patientRepo, accessRepo: accessRepo}
}

func (c *checker) RequireAccess(ctx context.Context, accountID, patientID uuid.UUID) error {
	_, err := c.ResolveAccessiblePatient(ctx, accountID, patientID)
	return err
}

func (c *checker) ResolveAccessiblePatient(
	ctx context.Context,
	accountID, patientID uuid.UUID,
) (*profiledomain.Patient, error) {
	if accountID == uuid.Nil {
		return nil, apperr.Unauthorized("autenticação necessária")
	}
	if patientID == uuid.Nil {
		return nil, apperr.Internal("erro inesperado", errors.New("patientID is required"))
	}
	if c.patientRepo == nil || c.accessRepo == nil {
		return nil, apperr.Internal("erro inesperado", errors.New("patient access repositories not configured"))
	}

	patient, err := c.patientRepo.FindByID(ctx, patientID)
	if err != nil {
		return nil, &apperr.AppError{
			Kind:    apperr.INFRA_DATABASE_ERROR,
			Message: "falha técnica",
			Cause:   fmt.Errorf("patientRepo.FindByID: %w", err),
		}
	}
	// A missing patient and an inaccessible patient produce the same external error.
	if patient == nil {
		return nil, apperr.Forbidden("acesso negado")
	}
	if patient.OwnerUserID != nil && *patient.OwnerUserID == accountID {
		return patient, nil
	}

	hasAccess, err := c.accessRepo.HasActiveAccess(ctx, patientID, accountID)
	if err != nil {
		return nil, &apperr.AppError{
			Kind:    apperr.INFRA_DATABASE_ERROR,
			Message: "falha técnica",
			Cause:   fmt.Errorf("accessRepo.HasActiveAccess: %w", err),
		}
	}
	if !hasAccess {
		return nil, apperr.Forbidden("acesso negado")
	}
	return patient, nil
}

var _ Checker = (*checker)(nil)
var _ Resolver = (*checker)(nil)
