// internal/application/bootstrap/account.go
package bootstrap

import (
	"github.com/gabrielgcmr/sonnda/internal/config"
	"github.com/gabrielgcmr/sonnda/internal/features/account"
	accounthttp "github.com/gabrielgcmr/sonnda/internal/features/account/http"
	accountpostgres "github.com/gabrielgcmr/sonnda/internal/features/account/postgres"
	accountredis "github.com/gabrielgcmr/sonnda/internal/features/account/redis"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/redis/go-redis/v9"
)

type AccountModule struct {
	Handler    *accounthttp.Handler
	Middleware *accounthttp.Middleware
}

func NewAccountModule(db *postgress.Client, redisClient *redis.Client, activationConfig config.ProfessionalActivationConfig) *AccountModule {
	userRepo := accountpostgres.New(db.Pool())

	service := account.New(userRepo)
	onboarding := account.NewOnboarding(userRepo, service)
	var limiter account.ActivationLimiter
	if redisClient != nil {
		limiter = accountredis.NewActivationLimiter(redisClient)
	}
	activation := account.NewProfessionalActivationService(userRepo, limiter, account.ProfessionalActivationOptions{
		PasswordHash: activationConfig.PasswordHash,
		MaxAttempts:  activationConfig.MaxAttempts,
		Window:       activationConfig.Window,
	})

	return &AccountModule{
		Handler:    accounthttp.NewHandler(onboarding, service, activation),
		Middleware: accounthttp.NewMiddleware(userRepo),
	}
}
