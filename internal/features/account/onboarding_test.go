// internal/features/account/onboarding_test.go
package account

import (
	"context"
	"testing"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
)

type onboardingRepository struct {
	Repository
}

func (r onboardingRepository) FindByAuthIdentity(context.Context, string, string) (*accountdomain.Account, error) {
	return nil, nil
}

type onboardingUserService struct {
	Service
	input AccountCreateInput
}

func (s *onboardingUserService) Create(_ context.Context, input AccountCreateInput) (*accountdomain.Account, error) {
	s.input = input
	return &accountdomain.Account{AccountType: input.AccountType, Profile: input.Profile}, nil
}

func TestOnboardingRegistersWithoutProfessionalProfile(t *testing.T) {
	for _, accountType := range []accountdomain.AccountType{
		accountdomain.AccountTypeProfessional,
		accountdomain.AccountTypeBasicCare,
	} {
		t.Run(string(accountType), func(t *testing.T) {
			email, name := "person@example.com", "Pessoa Teste"
			service := &onboardingUserService{}
			onboarding := NewOnboarding(onboardingRepository{}, service)
			created, err := onboarding.Register(context.Background(), RegisterInput{
				Issuer: "issuer", Subject: "subject", Email: &email,
				Profile: accountdomain.Profile{FullName: &name}, AccountType: accountType,
			})
			if err != nil {
				t.Fatalf("register: %v", err)
			}
			if created.AccountType != accountType || created.Profile.FullName == nil || *created.Profile.FullName != "Pessoa Teste" {
				t.Fatalf("unexpected user: %+v", created)
			}
			if service.input.Issuer != "issuer" || service.input.Subject != "subject" || service.input.Email == nil || *service.input.Email != "person@example.com" {
				t.Fatalf("identity was not forwarded: %+v", service.input)
			}
		})
	}
}
