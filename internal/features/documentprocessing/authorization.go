// internal/features/documentprocessing/authorization.go
package documentprocessing

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	domain "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type documentPatientAccessChecker interface {
	RequireAccess(context.Context, uuid.UUID, uuid.UUID) error
}

type Authorizer interface {
	AuthorizeStandalone(*accountdomain.Account, Action) error
	AuthorizePatient(context.Context, *accountdomain.Account, uuid.UUID, Action) error
	AuthorizeDocument(context.Context, *accountdomain.Account, uuid.UUID, Action) (*domain.ExamDocument, error)
}

type authorizer struct {
	documents DocumentRepository
	access    documentPatientAccessChecker
}

func NewAuthorizer(documents DocumentRepository, access documentPatientAccessChecker) Authorizer {
	return &authorizer{documents: documents, access: access}
}

func (a *authorizer) AuthorizeStandalone(currentAccount *accountdomain.Account, action Action) error {
	actor, err := documentActor(currentAccount, uuid.Nil)
	if err != nil {
		return err
	}
	return RequireAction(action, uuid.Nil, actor)
}

func (a *authorizer) AuthorizePatient(ctx context.Context, currentAccount *accountdomain.Account, patientID uuid.UUID, action Action) error {
	actor, err := documentActor(currentAccount, patientID)
	if err != nil {
		return err
	}
	if !patientScopedAction(action) {
		return apperr.Forbidden("acesso negado")
	}
	if a == nil || a.access == nil {
		return apperr.Internal("erro inesperado", errors.New("document authorizer access dependency not configured"))
	}
	if err := a.access.RequireAccess(ctx, actor.AccountID, patientID); err != nil {
		return err
	}
	actor.HasAccess = true
	return RequireAction(action, patientID, actor)
}

func (a *authorizer) AuthorizeDocument(ctx context.Context, currentAccount *accountdomain.Account, documentID uuid.UUID, action Action) (*domain.ExamDocument, error) {
	if documentID == uuid.Nil {
		return nil, apperr.Validation("entrada inválida", apperr.Violation{Field: "id", Reason: "required"})
	}
	if _, err := documentActor(currentAccount, uuid.Nil); err != nil {
		return nil, err
	}
	if !patientScopedAction(action) {
		return nil, apperr.Forbidden("acesso negado")
	}
	if a == nil || a.documents == nil {
		return nil, apperr.Internal("erro inesperado", errors.New("document authorizer repository dependency not configured"))
	}
	document, err := a.documents.FindByID(ctx, documentID)
	if err != nil {
		return nil, mapRepoError("exams.find_by_id", err)
	}
	if document == nil {
		return nil, nil
	}
	if err := a.AuthorizePatient(ctx, currentAccount, document.PatientID, action); err != nil {
		return nil, err
	}
	return document, nil
}

func documentActor(currentAccount *accountdomain.Account, patientID uuid.UUID) (authz.PatientContext, error) {
	if currentAccount == nil || currentAccount.ID == uuid.Nil {
		return authz.PatientContext{}, apperr.Unauthorized("autenticação necessária")
	}
	if currentAccount.DeletedAt != nil || !currentAccount.AccountType.IsValid() {
		return authz.PatientContext{}, apperr.Forbidden("acesso negado")
	}
	return authz.PatientContext{AccountID: currentAccount.ID, PatientID: patientID, AccountType: currentAccount.AccountType}, nil
}

var _ Authorizer = (*authorizer)(nil)
