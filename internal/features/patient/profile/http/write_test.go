// internal/features/patient/profile/http/write_test.go
package profilehttp

import (
	"context"
	"net/http"
	"testing"

	"github.com/gabrielgcmr/sonnda/internal/domain/demographics"
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	profiledomain "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/domain"
	"github.com/google/uuid"
)

type patientServiceStub struct {
	patient     *profiledomain.Patient
	updateID    uuid.UUID
	updateInput patientprofile.UpdateInput
	updateCalls int
	deleteID    uuid.UUID
	deleteCalls int
}

func (s *patientServiceStub) Get(context.Context, *accountdomain.Account, uuid.UUID) (*profiledomain.Patient, error) {
	return s.patient, nil
}

func (s *patientServiceStub) Update(
	_ context.Context,
	_ *accountdomain.Account,
	id uuid.UUID,
	input patientprofile.UpdateInput,
) (*profiledomain.Patient, error) {
	s.updateCalls++
	s.updateID = id
	s.updateInput = input
	return s.patient, nil
}

func (s *patientServiceStub) SoftDelete(_ context.Context, _ *accountdomain.Account, id uuid.UUID) error {
	s.deleteCalls++
	s.deleteID = id
	return nil
}

func TestPatchPatientUpdatesOnlySuppliedFields(t *testing.T) {
	patientID := uuid.New()
	svc := &patientServiceStub{patient: &profiledomain.Patient{ID: patientID, FullName: "Maria Silva"}}
	response := performPatientRequest(t, svc, nil, http.MethodPatch, "/patients/"+patientID.String(), `{
		"full_name":"Maria Silva",
		"phone":null,
		"gender":"FEMALE",
		"race":"WHITE"
	}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if svc.updateCalls != 1 || svc.updateID != patientID {
		t.Fatalf("unexpected update call: calls=%d id=%s", svc.updateCalls, svc.updateID)
	}
	if svc.updateInput.FullName == nil || *svc.updateInput.FullName != "Maria Silva" {
		t.Fatalf("full name was not forwarded: %+v", svc.updateInput)
	}
	if svc.updateInput.Phone == nil || *svc.updateInput.Phone != "" {
		t.Fatalf("null phone was not translated to clear: %+v", svc.updateInput)
	}
	if svc.updateInput.Gender == nil || *svc.updateInput.Gender != demographics.GenderFemale ||
		svc.updateInput.Race == nil || *svc.updateInput.Race != demographics.RaceWhite {
		t.Fatalf("demographics were not parsed: %+v", svc.updateInput)
	}
	if svc.updateInput.AvatarURL != nil || svc.updateInput.CNS != nil {
		t.Fatalf("omitted fields were forwarded: %+v", svc.updateInput)
	}
}

func TestPatchPatientRejectsEmptyOrInvalidUpdates(t *testing.T) {
	patientID := uuid.New()
	for _, body := range []string{`{}`, `{"gender":"invalid"}`, `{"full_name":null}`} {
		t.Run(body, func(t *testing.T) {
			svc := &patientServiceStub{patient: &profiledomain.Patient{ID: patientID}}
			response := performPatientRequest(t, svc, nil, http.MethodPatch, "/patients/"+patientID.String(), body)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
			}
			if svc.updateCalls != 0 {
				t.Fatal("service called for an invalid update")
			}
		})
	}
}

func TestDeletePatientUsesSoftDelete(t *testing.T) {
	patientID := uuid.New()
	svc := &patientServiceStub{}
	response := performPatientRequest(t, svc, nil, http.MethodDelete, "/patients/"+patientID.String(), "")

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if svc.deleteCalls != 1 || svc.deleteID != patientID {
		t.Fatalf("unexpected delete call: calls=%d id=%s", svc.deleteCalls, svc.deleteID)
	}
}
