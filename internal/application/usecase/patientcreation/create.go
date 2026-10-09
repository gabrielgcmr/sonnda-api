// internal/application/usecase/patientcreation/create.go
package patientcreation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accessdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/access/domain"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"

	"github.com/google/uuid"
)

type AccessInput struct {
	RelationType string
}

type Input struct {
	Profile patientprofile.CreateInput
	Access  AccessInput
}

type Repository interface {
	CreateWithInitialAccess(
		ctx context.Context,
		patient *profiledomain.Patient,
		access *accessdomain.PatientAccess,
	) error
}

type UseCase interface {
	Execute(ctx context.Context, creatorAccount *accountdomain.Account, input Input) (*profiledomain.Patient, error)
}

type useCase struct {
	repo Repository
}

func New(repo Repository) UseCase {
	return &useCase{repo: repo}
}

func (u *useCase) Execute(
	ctx context.Context,
	creatorAccount *accountdomain.Account,
	input Input,
) (*profiledomain.Patient, error) {
	relationType := accessdomain.RelationshipType(strings.TrimSpace(input.Access.RelationType))
	if err := RequireInitialRelationship(creatorAccount, relationType); err != nil {
		return nil, err
	}
	if !relationType.IsValid() {
		return nil, apperr.Validation(
			"vínculo com paciente inválido",
			apperr.Violation{Field: "relation_type", Reason: "required or invalid"},
		)
	}
	if u.repo == nil {
		return nil, apperr.Internal("erro inesperado", errors.New("patient creation repository not configured"))
	}

	creatorAccountID := creatorAccount.ID

	var ownerAccountID *uuid.UUID
	if relationType == accessdomain.RelationshipTypeSelf {
		ownerAccountID = &creatorAccountID
	}

	patient, err := profiledomain.NewPatient(profiledomain.NewPatientParams{
		UserID:    ownerAccountID,
		CPF:       input.Profile.CPF,
		CNS:       input.Profile.CNS,
		FullName:  input.Profile.FullName,
		BirthDate: input.Profile.BirthDate,
		Gender:    input.Profile.Gender,
		Race:      input.Profile.Race,
		Phone:     input.Profile.Phone,
		AvatarURL: input.Profile.AvatarURL,
	})
	if err != nil {
		return nil, mapProfileError(err)
	}

	initialAccess, err := accessdomain.NewPatientAccess(
		patient.ID,
		creatorAccountID,
		relationType,
		&creatorAccountID,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, apperr.Validation(
			"vínculo com paciente inválido",
			apperr.Violation{Field: "relation_type", Reason: "invalid"},
		)
	}

	if err := u.repo.CreateWithInitialAccess(ctx, patient, initialAccess); err != nil {
		return nil, mapRepositoryError(err)
	}

	return patient, nil
}

func mapProfileError(err error) error {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return apperr.Validation("dados do paciente inválidos")
}

func mapRepositoryError(err error) error {
	switch {
	case errors.Is(err, patientprofile.ErrPatientAlreadyExists):
		return apperr.AlreadyExists("paciente já cadastrado")
	case errors.Is(err, persistence.ErrPersistenceFailure):
		return apperr.Internal("falha técnica", fmt.Errorf("create patient with initial access: %w", err))
	default:
		return apperr.Internal("erro inesperado", fmt.Errorf("create patient with initial access: %w", err))
	}
}

var _ UseCase = (*useCase)(nil)
