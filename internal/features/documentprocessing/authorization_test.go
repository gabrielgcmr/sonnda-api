// internal/features/documentprocessing/authorization_test.go
package documentprocessing

import (
	"context"
	"errors"
	"testing"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	domain "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type documentAccessRecorder struct {
	err       error
	calls     int
	accountID uuid.UUID
	patientID uuid.UUID
}

func (a *documentAccessRecorder) RequireAccess(_ context.Context, accountID, patientID uuid.UUID) error {
	a.calls++
	a.accountID, a.patientID = accountID, patientID
	return a.err
}

type documentStoreRecorder struct {
	DocumentRepository
	document *domain.ExamDocument
	calls    int
}

func (s *documentStoreRecorder) FindByID(context.Context, uuid.UUID) (*domain.ExamDocument, error) {
	s.calls++
	return s.document, nil
}

func TestStandaloneExtractionRequiresOnlyAValidAccount(t *testing.T) {
	for _, accountType := range []accountdomain.AccountType{accountdomain.AccountTypeBasicCare, accountdomain.AccountTypeProfessional} {
		t.Run(string(accountType), func(t *testing.T) {
			account := &accountdomain.Account{ID: uuid.New(), AccountType: accountType}
			store := &documentStoreRecorder{}
			access := &documentAccessRecorder{}
			err := NewAuthorizer(store, access).AuthorizeStandalone(account, ExtractStandaloneLab)
			if err != nil {
				t.Fatalf("unexpected standalone denial: %v", err)
			}
			if store.calls != 0 || access.calls != 0 {
				t.Fatalf("standalone touched patient resources: documents=%d access=%d", store.calls, access.calls)
			}
		})
	}
}

func TestPatientDocumentActionsRequirePatientAccess(t *testing.T) {
	patientID := uuid.New()
	account := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}
	for _, action := range []Action{ListPatientDocuments, ListPatientTexts, UploadDocument} {
		t.Run(string(action), func(t *testing.T) {
			access := &documentAccessRecorder{}
			err := NewAuthorizer(nil, access).AuthorizePatient(t.Context(), account, patientID, action)
			if err != nil {
				t.Fatalf("unexpected authorization error: %v", err)
			}
			if access.calls != 1 || access.accountID != account.ID || access.patientID != patientID {
				t.Fatalf("unexpected access check: %+v", access)
			}
		})
	}
}

func TestDocumentActionsLoadOnceAndAuthorizeOwningPatient(t *testing.T) {
	patientID := uuid.New()
	document := &domain.ExamDocument{ID: uuid.New(), PatientID: patientID}
	account := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional}
	for _, action := range []Action{ReadDocument, ReadExtraction, ConfirmDocument, DiscardDocument} {
		t.Run(string(action), func(t *testing.T) {
			store := &documentStoreRecorder{document: document}
			access := &documentAccessRecorder{}
			result, err := NewAuthorizer(store, access).AuthorizeDocument(t.Context(), account, document.ID, action)
			if err != nil || result != document {
				t.Fatalf("authorization failed: result=%p document=%p err=%v", result, document, err)
			}
			if store.calls != 1 || access.calls != 1 || access.patientID != patientID {
				t.Fatalf("unexpected calls: documents=%d access=%d patient=%s", store.calls, access.calls, access.patientID)
			}
		})
	}
}

func TestDocumentAuthorizerRejectsInvalidAccountWithoutPatientQueries(t *testing.T) {
	deletedAt := time.Now()
	tests := []struct {
		name    string
		account *accountdomain.Account
		kind    apperr.ErrorKind
	}{
		{name: "missing", account: nil, kind: apperr.AUTH_REQUIRED},
		{name: "deleted", account: &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional, DeletedAt: &deletedAt}, kind: apperr.ACCESS_DENIED},
		{name: "invalid type", account: &accountdomain.Account{ID: uuid.New(), AccountType: "invalid"}, kind: apperr.ACCESS_DENIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &documentStoreRecorder{}
			access := &documentAccessRecorder{}
			err := NewAuthorizer(store, access).AuthorizeStandalone(tt.account, ExtractStandaloneLab)
			assertDocumentErrorKind(t, err, tt.kind)
			if store.calls != 0 || access.calls != 0 {
				t.Fatalf("patient resources called: documents=%d access=%d", store.calls, access.calls)
			}
		})
	}
}

func TestDocumentAuthorizerPropagatesPatientAccessDenial(t *testing.T) {
	denied := apperr.Forbidden("acesso negado")
	access := &documentAccessRecorder{err: denied}
	account := &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeProfessional}
	err := NewAuthorizer(nil, access).AuthorizePatient(t.Context(), account, uuid.New(), UploadDocument)
	if !errors.Is(err, denied) {
		t.Fatalf("expected access denial, got %v", err)
	}
}

func assertDocumentErrorKind(t *testing.T, err error, want apperr.ErrorKind) {
	t.Helper()
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Kind != want {
		t.Fatalf("expected %s, got %v", want, err)
	}
}
