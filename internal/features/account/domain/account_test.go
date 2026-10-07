// internal/features/account/domain/account_test.go
package accountdomain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func ptr[T any](value T) *T { return &value }

func TestMinimalAccountIsValidWithoutIdentityOrProfile(t *testing.T) {
	a, err := NewAccount(NewAccountParams{})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == uuid.Nil || a.AccountType != AccountTypeBasicCare || a.DeletedAt != nil {
		t.Fatalf("unexpected account: %+v", a)
	}
	if a.Profile != (Profile{}) || a.OnboardingCompleted() {
		t.Fatalf("unexpected profile: %+v", a.Profile)
	}
	if a.CreatedAt.IsZero() || a.CreatedAt.Location() != time.UTC || !a.CreatedAt.Equal(a.UpdatedAt) {
		t.Fatal("creation timestamps must be the same UTC instant")
	}
}

func TestOnboardingIsDerivedFromNameAndBirthDate(t *testing.T) {
	a, _ := NewAccount(NewAccountParams{})
	name := "Ana Silva"
	birth := time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		profile  Profile
		complete bool
	}{
		{Profile{FullName: &name}, false},
		{Profile{BirthDate: &birth}, false},
		{Profile{FullName: &name, BirthDate: &birth}, true},
		{Profile{FullName: &name}, false},
		{Profile{BirthDate: &birth}, false},
		{Profile{}, false},
	} {
		if _, err := a.ApplyProfile(tc.profile); err != nil {
			t.Fatal(err)
		}
		if a.OnboardingCompleted() != tc.complete {
			t.Fatalf("profile=%+v completion=%v", a.Profile, a.OnboardingCompleted())
		}
	}
	for _, p := range []Profile{
		{FullName: ptr("X"), BirthDate: &birth},
		{FullName: &name, BirthDate: ptr(time.Time{})},
		{FullName: &name, BirthDate: ptr(time.Now().UTC().AddDate(0, 0, 1))},
	} {
		if p.OnboardingCompleted() {
			t.Fatal("invalid name or birth date completed onboarding")
		}
	}
}

func TestProfileValidationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile Profile
		want    error
	}{
		{"missing values", Profile{}, nil},
		{"empty name", Profile{FullName: ptr("   ")}, ErrInvalidFullName},
		{"short name", Profile{FullName: ptr("A")}, ErrInvalidFullName},
		{"minimum name", Profile{FullName: ptr("Áá")}, nil},
		{"maximum unicode name", Profile{FullName: ptr(strings.Repeat("á", 120))}, nil},
		{"long name", Profile{FullName: ptr(strings.Repeat("á", 121))}, ErrInvalidFullName},
		{"zero date", Profile{BirthDate: ptr(time.Time{})}, ErrInvalidBirthDate},
		{"future date", Profile{BirthDate: ptr(time.Now().UTC().AddDate(0, 0, 1))}, ErrInvalidBirthDate},
		{"today", Profile{BirthDate: ptr(time.Now().UTC())}, nil},
		{"blank CPF", Profile{CPF: ptr(" ")}, nil},
		{"formatted CPF", Profile{CPF: ptr("123.456.789-01")}, nil},
		{"no CPF checksum rule", Profile{CPF: ptr("00000000000")}, nil},
		{"short CPF", Profile{CPF: ptr("123")}, ErrInvalidCPF},
		{"long CPF", Profile{CPF: ptr("123456789012")}, ErrInvalidCPF},
		{"invalid CPF", Profile{CPF: ptr("abc")}, ErrInvalidCPF},
		{"blank phone", Profile{Phone: ptr(" ")}, nil},
		{"minimum phone", Profile{Phone: ptr("1234567890")}, nil},
		{"maximum phone", Profile{Phone: ptr("+123456789012345")}, nil},
		{"short phone", Profile{Phone: ptr("123456789")}, ErrInvalidPhone},
		{"long phone", Profile{Phone: ptr("1234567890123456")}, ErrInvalidPhone},
		{"non-digit phone", Profile{Phone: ptr("12345abcde")}, ErrInvalidPhone},
		{"misplaced plus", Profile{Phone: ptr("12345+67890")}, ErrInvalidPhone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAccount(NewAccountParams{Profile: tc.profile})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestProfileUpdatesAreAtomicAndDoNotAliasInputs(t *testing.T) {
	name := "  Ana Silva  "
	birth := time.Date(1990, 1, 2, 23, 0, 0, 0, time.FixedZone("BRT", -3*3600))
	a, err := NewAccount(NewAccountParams{Profile: Profile{FullName: &name, BirthDate: &birth, CPF: ptr("123.456.789-01"), Phone: ptr(" 11999999999 ")}})
	if err != nil {
		t.Fatal(err)
	}
	if *a.Profile.FullName != "Ana Silva" || *a.Profile.CPF != "12345678901" || a.Profile.BirthDate.Format(time.DateOnly) != "1990-01-02" {
		t.Fatalf("unexpected normalization: %+v", a.Profile)
	}
	name = "Mutated input"
	if *a.Profile.FullName != "Ana Silva" {
		t.Fatal("profile aliased input")
	}
	before := *a
	changed, err := a.ApplyProfile(Profile{FullName: ptr("Nome Novo"), Phone: ptr("invalid")})
	if changed || !errors.Is(err, ErrInvalidPhone) || *a != before {
		t.Fatal("failed update mutated account")
	}
	changed, err = a.ApplyProfile(a.Profile)
	if changed || err != nil || !a.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("equivalent update changed timestamp")
	}
	profile := a.Profile
	profile.CPF, profile.Phone = ptr(""), ptr(" ")
	if changed, err = a.ApplyProfile(profile); !changed || err != nil {
		t.Fatalf("clear: %v %v", changed, err)
	}
	if a.Profile.CPF != nil || a.Profile.Phone != nil || !a.OnboardingCompleted() {
		t.Fatal("optional field clearing changed onboarding")
	}
}

func TestAccountTypeValidation(t *testing.T) {
	if _, err := NewAccount(NewAccountParams{AccountType: "invalid"}); !errors.Is(err, ErrInvalidAccountType) {
		t.Fatal(err)
	}
}

func TestIdentityIsSeparateAndEmailIsOptional(t *testing.T) {
	id := uuid.New()
	i, err := NewIdentity(id, "issuer", "subject", nil)
	if err != nil || i.AccountID != id || i.Email != nil {
		t.Fatalf("identity=%+v err=%v", i, err)
	}
	i2, err := NewIdentity(id, "another-issuer", "subject", ptr(" "))
	if err != nil || i2.Email != nil || i2.Issuer == i.Issuer {
		t.Fatal("identity normalization lost issuer or optional email")
	}
	for _, tc := range []struct {
		id              uuid.UUID
		issuer, subject string
		want            error
	}{
		{uuid.Nil, "issuer", "subject", ErrInvalidAccountID},
		{id, " ", "subject", ErrInvalidAuthIssuer},
		{id, "issuer", " ", ErrInvalidAuthSubject},
	} {
		if _, err := NewIdentity(tc.id, tc.issuer, tc.subject, nil); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
	}
}
