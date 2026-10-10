// internal/features/patient/exam/laboratory/authorization.go
package laboratory

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	labdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type patientAccessChecker interface {
	RequireAccess(context.Context, uuid.UUID, uuid.UUID) error
}

type Authorizer interface {
	AuthorizePatient(context.Context, *accountdomain.Account, uuid.UUID, Action) error
	AuthorizeReport(context.Context, *accountdomain.Account, uuid.UUID, Action) (*labdomain.LabReport, error)
}

type authorizer struct {
	reports Repository
	access  patientAccessChecker
}

func NewAuthorizer(reports Repository, access patientAccessChecker) Authorizer {
	return &authorizer{reports: reports, access: access}
}

func (a *authorizer) AuthorizePatient(ctx context.Context, currentAccount *accountdomain.Account, patientID uuid.UUID, action Action) error {
	actor, err := laboratoryActor(currentAccount, patientID)
	if err != nil {
		return err
	}
	if !patientAction(action) {
		return apperr.Forbidden("acesso negado")
	}
	if a == nil || a.access == nil {
		return apperr.Internal("erro inesperado", errors.New("laboratory authorizer access dependency not configured"))
	}
	if err := a.access.RequireAccess(ctx, actor.AccountID, patientID); err != nil {
		return err
	}
	actor.HasAccess = true
	return RequireAction(action, patientID, actor)
}

func (a *authorizer) AuthorizeReport(ctx context.Context, currentAccount *accountdomain.Account, reportID uuid.UUID, action Action) (*labdomain.LabReport, error) {
	if reportID == uuid.Nil {
		return nil, apperr.Validation("entrada inválida", apperr.Violation{Field: "id", Reason: "required"})
	}
	if _, err := laboratoryActor(currentAccount, uuid.Nil); err != nil {
		return nil, err
	}
	if action != ReadReport {
		return nil, apperr.Forbidden("acesso negado")
	}
	if a == nil || a.reports == nil {
		return nil, apperr.Internal("erro inesperado", errors.New("laboratory authorizer report dependency not configured"))
	}
	report, err := a.reports.FindByID(ctx, reportID)
	if err != nil {
		return nil, mapRepoError("labs.find_by_id", err)
	}
	if report == nil {
		return nil, nil
	}
	if err := a.AuthorizePatient(ctx, currentAccount, report.PatientID, action); err != nil {
		return nil, err
	}
	return report, nil
}

func laboratoryActor(currentAccount *accountdomain.Account, patientID uuid.UUID) (authz.PatientContext, error) {
	if currentAccount == nil || currentAccount.ID == uuid.Nil {
		return authz.PatientContext{}, apperr.Unauthorized("autenticação necessária")
	}
	if currentAccount.DeletedAt != nil || !currentAccount.AccountType.IsValid() {
		return authz.PatientContext{}, apperr.Forbidden("acesso negado")
	}
	return authz.PatientContext{AccountID: currentAccount.ID, PatientID: patientID, AccountType: currentAccount.AccountType}, nil
}

var _ Authorizer = (*authorizer)(nil)
