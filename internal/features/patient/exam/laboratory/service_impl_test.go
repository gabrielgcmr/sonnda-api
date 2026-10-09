// internal/features/patient/exam/laboratory/service_impl_test.go
package laboratory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	labs "github.com/gabrielgcmr/sonnda/internal/features/patient/exam/laboratory/domain"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gabrielgcmr/sonnda/internal/kernel/persistence"

	"github.com/google/uuid"
)

type fakePatientRepo struct {
	findByIDRes *profiledomain.Patient
	findByIDErr error
}

func (r *fakePatientRepo) Create(ctx context.Context, p *profiledomain.Patient) error {
	panic("unused")
}
func (r *fakePatientRepo) Update(ctx context.Context, p *profiledomain.Patient) error {
	panic("unused")
}
func (r *fakePatientRepo) SoftDelete(ctx context.Context, id uuid.UUID) error { panic("unused") }
func (r *fakePatientRepo) HardDelete(ctx context.Context, id uuid.UUID) error { panic("unused") }
func (r *fakePatientRepo) FindByCPF(ctx context.Context, cpf string) (*profiledomain.Patient, error) {
	panic("unused")
}
func (r *fakePatientRepo) FindByID(ctx context.Context, id uuid.UUID) (*profiledomain.Patient, error) {
	return r.findByIDRes, r.findByIDErr
}
func (r *fakePatientRepo) FindByName(ctx context.Context, name string) ([]profiledomain.Patient, error) {
	panic("unused")
}
func (r *fakePatientRepo) List(ctx context.Context, limit, offset int) ([]profiledomain.Patient, error) {
	panic("unused")
}
func (r *fakePatientRepo) SearchByName(ctx context.Context, name string, limit, offset int) ([]profiledomain.Patient, error) {
	panic("unused")
}

type fakeLabsRepo struct {
	listRes []labs.LabReport
	listErr error
}

type fakeLaboratoryAuthorizer struct {
	err error
}

func (a *fakeLaboratoryAuthorizer) AuthorizePatient(context.Context, *accountdomain.Account, uuid.UUID, Action) error {
	return a.err
}

func (a *fakeLaboratoryAuthorizer) AuthorizeReport(context.Context, *accountdomain.Account, uuid.UUID, Action) (*labs.LabReport, error) {
	return nil, a.err
}

func laboratoryTestAccount() *accountdomain.Account {
	return &accountdomain.Account{ID: uuid.New(), AccountType: accountdomain.AccountTypeBasicCare}
}

func (r *fakeLabsRepo) Create(ctx context.Context, report *labs.LabReport) error { panic("unused") }
func (r *fakeLabsRepo) AttachDocument(ctx context.Context, reportID, patientID, documentID uuid.UUID) error {
	panic("unused")
}
func (r *fakeLabsRepo) ExistsBySignature(ctx context.Context, patientID uuid.UUID, fingerprint string) (bool, error) {
	panic("unused")
}
func (r *fakeLabsRepo) Delete(ctx context.Context, id uuid.UUID) error { panic("unused") }
func (r *fakeLabsRepo) FindByID(ctx context.Context, reportID uuid.UUID) (*labs.LabReport, error) {
	panic("unused")
}
func (r *fakeLabsRepo) FindBySignature(ctx context.Context, patientID uuid.UUID, fingerprint string) (*labs.LabReport, error) {
	panic("unused")
}
func (r *fakeLabsRepo) ListLabs(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]labs.LabReport, error) {
	return r.listRes, r.listErr
}
func (r *fakeLabsRepo) ListObservationTimelineByPatientAndParameter(
	ctx context.Context,
	patientID uuid.UUID,
	parameterName string,
	limit, offset int,
) ([]labs.ObservationTimeline, error) {
	panic("unused")
}

func TestList_InvalidPatientID_ReturnsValidationFailed(t *testing.T) {
	svc := New(&fakeLabsRepo{}, &fakeLaboratoryAuthorizer{})

	_, err := svc.List(context.Background(), laboratoryTestAccount(), uuid.Nil, 10, 0)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Kind != apperr.VALIDATION_FAILED {
		t.Fatalf("expected VALIDATION_FAILED, got %s", appErr.Kind)
	}
}

func TestList_PatientNotFound_ReturnsNotFound(t *testing.T) {
	svc := New(&fakeLabsRepo{}, &fakeLaboratoryAuthorizer{err: patientNotFound()})

	_, err := svc.List(context.Background(), laboratoryTestAccount(), uuid.Must(uuid.NewV7()), 10, 0)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Kind != apperr.NOT_FOUND {
		t.Fatalf("expected NOT_FOUND, got %s", appErr.Kind)
	}
}

func TestList_PatientRepoError_ReturnsInfraDatabaseError(t *testing.T) {
	sentinel := errors.New("db down")
	svc := New(&fakeLabsRepo{}, &fakeLaboratoryAuthorizer{err: mapRepoError("patient.find_by_id", errors.Join(persistence.ErrPersistenceFailure, sentinel))})

	_, err := svc.List(context.Background(), laboratoryTestAccount(), uuid.Must(uuid.NewV7()), 10, 0)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Kind != apperr.INFRA_DATABASE_ERROR {
		t.Fatalf("expected INFRA_DATABASE_ERROR, got %s", appErr.Kind)
	}
}

func TestList_LabsRepoError_ReturnsInfraDatabaseError(t *testing.T) {
	svc := New(
		&fakeLabsRepo{listErr: errors.New("db down")},
		&fakeLaboratoryAuthorizer{},
	)

	_, err := svc.List(context.Background(), laboratoryTestAccount(), uuid.Must(uuid.NewV7()), 10, 0)

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Kind != apperr.INFRA_DATABASE_ERROR {
		t.Fatalf("expected INFRA_DATABASE_ERROR, got %s", appErr.Kind)
	}
}

func TestLabReportOutputUsesPanelAndObservationJSONNames(t *testing.T) {
	report := LabReportOutput{
		Panels: []LabPanelOutput{{
			Observations: []ObservationOutput{{ParameterName: "Glicose"}},
		}},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["panels"]; !ok {
		t.Fatal("report output is missing panels")
	}
	if _, ok := body["test_results"]; ok {
		t.Fatal("report output still exposes test_results")
	}

	var panels []map[string]json.RawMessage
	if err := json.Unmarshal(body["panels"], &panels); err != nil {
		t.Fatal(err)
	}
	if _, ok := panels[0]["observations"]; !ok {
		t.Fatal("panel output is missing observations")
	}
	if _, ok := panels[0]["items"]; ok {
		t.Fatal("panel output still exposes items")
	}
}
