// internal/features/documentprocessing/service.go
package documentprocessing

import (
	"context"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/google/uuid"
)

type Service interface {
	FindByID(ctx context.Context, currentAccount *accountdomain.Account, id uuid.UUID) (*ExamDocumentOutput, error)
	ListByPatient(ctx context.Context, currentAccount *accountdomain.Account, patientID uuid.UUID, limit, offset int) ([]ExamDocumentOutput, error)
	ListDocumentTextsByPatient(ctx context.Context, currentAccount *accountdomain.Account, patientID uuid.UUID, limit, offset int) ([]ExamDocumentTextOutput, error)
}
