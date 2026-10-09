// internal/features/documentprocessing/policy.go
package documentprocessing

import (
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

type Action string

const (
	ExtractStandaloneLab Action = "extract_standalone_lab"
	ListPatientDocuments Action = "list_patient_documents"
	ListPatientTexts     Action = "list_patient_document_texts"
	ReadDocument         Action = "read_document"
	ReadExtraction       Action = "read_document_extraction"
	UploadDocument       Action = "upload_document"
	ConfirmDocument      Action = "confirm_document"
	DiscardDocument      Action = "discard_document"
)

func RequireAction(action Action, patientID uuid.UUID, actor authz.PatientContext) error {
	if actor.AccountID == uuid.Nil {
		return apperr.Unauthorized("autenticação necessária")
	}
	if !actor.AccountType.IsValid() {
		return apperr.Forbidden("acesso negado")
	}
	if action == ExtractStandaloneLab {
		return nil
	}
	if patientID == uuid.Nil || actor.PatientID != patientID || !actor.HasAccess {
		return apperr.Forbidden("acesso negado")
	}
	switch action {
	case ListPatientDocuments, ListPatientTexts, ReadDocument, ReadExtraction, UploadDocument, ConfirmDocument, DiscardDocument:
		return nil
	default:
		return apperr.Forbidden("acesso negado")
	}
}

func patientScopedAction(action Action) bool {
	switch action {
	case ListPatientDocuments, ListPatientTexts, ReadDocument, ReadExtraction, UploadDocument, ConfirmDocument, DiscardDocument:
		return true
	default:
		return false
	}
}
