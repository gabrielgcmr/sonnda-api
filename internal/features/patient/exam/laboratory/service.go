// internal/features/patient/exam/laboratory/service.go
package laboratory

import (
	"context"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/google/uuid"
)

type Service interface {
	List(ctx context.Context, currentAccount *accountdomain.Account, patientID uuid.UUID, limit, offset int) ([]LabReportSummaryOutput, error)
	ListFull(ctx context.Context, currentAccount *accountdomain.Account, patientID uuid.UUID, limit, offset int) ([]*LabReportOutput, error)
	FindByID(ctx context.Context, currentAccount *accountdomain.Account, reportID uuid.UUID) (*LabReportOutput, error)
}
