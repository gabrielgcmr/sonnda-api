// internal/features/patient/profile/domain/patient_test.go
package profiledomain

import (
	"errors"
	"testing"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/domain/demographics"

	"github.com/google/uuid"
)

func TestNewPatient_Success_NormalizesAndSetsUTC(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	cns := " 174 5984 3528 0018 "
	phone := " 11999999999 "
	birthDate := time.Now().Add(-24 * time.Hour)

	p, err := NewPatient(NewPatientParams{
		UserID:    &userID,
		CPF:       "52998224725",
		CNS:       &cns,
		FullName:  "  Paciente Teste  ",
		BirthDate: birthDate,
		Gender:    demographics.Gender("female"),
		Race:      demographics.Race("white"),
		Phone:     &phone,
		AvatarURL: "  https://example.com/a.png  ",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if p == nil {
		t.Fatalf("expected patient, got nil")
	}
	if p.FullName != "Paciente Teste" {
		t.Fatalf("expected full name trimmed")
	}
	if p.AvatarURL != "https://example.com/a.png" {
		t.Fatalf("expected avatar url trimmed")
	}
	if p.CreatedAt.Location() != time.UTC {
		t.Fatalf("expected CreatedAt in UTC")
	}
	if p.UpdatedAt.Location() != time.UTC {
		t.Fatalf("expected UpdatedAt in UTC")
	}
	if p.CPF == "" {
		t.Fatalf("expected CPF to be set")
	}
	if p.OwnerUserID == nil || *p.OwnerUserID != userID {
		t.Fatalf("expected UserID to be present")
	}
	if p.CNS == nil || *p.CNS != "174598435280018" {
		t.Fatalf("expected CNS to be present")
	}
	if p.Phone == nil || *p.Phone != "11999999999" {
		t.Fatalf("expected phone to be present")
	}
}

func TestNewPatient_InvalidInputs(t *testing.T) {
	now := time.Now()
	birthDate := now.Add(-24 * time.Hour)

	cases := []struct {
		name string
		err  error
		fn   func() NewPatientParams
	}{
		{
			name: "missing fullName",
			err:  demographics.ErrInvalidFullName,
			fn: func() NewPatientParams {
				params := validParams(birthDate)
				params.FullName = "   "
				return params
			},
		},
		{
			name: "invalid cpf",
			err:  demographics.ErrInvalidCPF,
			fn: func() NewPatientParams {
				params := validParams(birthDate)
				params.CPF = "123"
				return params
			},
		},
		{
			name: "invalid cpf check digits",
			err:  demographics.ErrInvalidCPF,
			fn: func() NewPatientParams {
				params := validParams(birthDate)
				params.CPF = "12345678901"
				return params
			},
		},
		{
			name: "repeated cpf",
			err:  demographics.ErrInvalidCPF,
			fn: func() NewPatientParams {
				params := validParams(birthDate)
				params.CPF = "00000000000"
				return params
			},
		},
		{
			name: "invalid cns",
			err:  demographics.ErrInvalidCNS,
			fn: func() NewPatientParams {
				params := validParams(birthDate)
				cns := "123456789012345"
				params.CNS = &cns
				return params
			},
		},
		{
			name: "non numeric cns",
			err:  demographics.ErrInvalidCNS,
			fn: func() NewPatientParams {
				params := validParams(birthDate)
				cns := "not-a-cns"
				params.CNS = &cns
				return params
			},
		},
		{
			name: "birthDate zero",
			err:  demographics.ErrInvalidBirthDate,
			fn: func() NewPatientParams {
				params := validParams(birthDate)
				params.BirthDate = time.Time{}
				return params
			},
		},
		{
			name: "birthDate future",
			err:  demographics.ErrInvalidBirthDate,
			fn: func() NewPatientParams {
				params := validParams(birthDate)
				params.BirthDate = now.Add(24 * time.Hour)
				return params
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewPatient(tc.fn())
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected %v, got %v", tc.err, err)
			}
		})
	}
}

func TestPatient_ApplyUpdateRejectsEmptyNameAtomically(t *testing.T) {
	p, err := NewPatient(NewPatientParams{
		CPF:       "52998224725",
		FullName:  "Paciente",
		BirthDate: time.Now().Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	before := p.UpdatedAt
	oldName, oldAvatar := p.FullName, p.AvatarURL

	emptyName := "   "
	newAvatar := "  http://img  "
	time.Sleep(2 * time.Millisecond)

	err = p.ApplyUpdate(&emptyName, nil, &newAvatar, nil, nil, nil)

	if !errors.Is(err, demographics.ErrInvalidFullName) {
		t.Fatalf("expected invalid full name, got %v", err)
	}
	if p.FullName != oldName || p.AvatarURL != oldAvatar {
		t.Fatalf("invalid update changed patient: name=%q avatar=%q", p.FullName, p.AvatarURL)
	}
	if !p.UpdatedAt.Equal(before) {
		t.Fatalf("invalid update changed UpdatedAt: before=%s after=%s", before, p.UpdatedAt)
	}
}

func TestPatient_ApplyUpdate_ClearsEmptyOptionalValues(t *testing.T) {
	cns := "174598435280018"
	phone := "11999999999"
	p, err := NewPatient(NewPatientParams{
		CPF:       "52998224725",
		CNS:       &cns,
		FullName:  "Paciente",
		BirthDate: time.Now().Add(-24 * time.Hour),
		Phone:     &phone,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	empty := "  "
	if err := p.ApplyUpdate(nil, &empty, nil, nil, nil, &empty); err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}

	if p.Phone != nil || p.CNS != nil {
		t.Fatalf("expected phone and CNS to be cleared: phone=%v CNS=%v", p.Phone, p.CNS)
	}
}

func TestPatientApplyUpdateValidatesAndNormalizesCNS(t *testing.T) {
	p, err := NewPatient(validParams(time.Now().Add(-24 * time.Hour)))
	if err != nil {
		t.Fatal(err)
	}

	formatted := "174 5984 3528 0018"
	if err := p.ApplyUpdate(nil, nil, nil, nil, nil, &formatted); err != nil {
		t.Fatalf("unexpected valid CNS error: %v", err)
	}
	if p.CNS == nil || *p.CNS != "174598435280018" {
		t.Fatalf("CNS was not normalized: %v", p.CNS)
	}

	before := *p.CNS
	invalid := "174598435280019"
	if err := p.ApplyUpdate(nil, nil, nil, nil, nil, &invalid); !errors.Is(err, demographics.ErrInvalidCNS) {
		t.Fatalf("expected invalid CNS, got %v", err)
	}
	if p.CNS == nil || *p.CNS != before {
		t.Fatalf("invalid CNS changed patient: %v", p.CNS)
	}
}

func validParams(birthDate time.Time) NewPatientParams {
	return NewPatientParams{
		CPF:       "52998224725",
		FullName:  "Paciente",
		BirthDate: birthDate,
		Gender:    demographics.GenderFemale,
		Race:      demographics.RaceUnknown,
	}
}
