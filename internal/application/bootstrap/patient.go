// internal/application/bootstrap/patient.go
package bootstrap

import (
	patientcreation "github.com/gabrielgcmr/sonnda/internal/application/usecase/patientcreation"
	accountpostgres "github.com/gabrielgcmr/sonnda/internal/features/account/postgres"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	accesspostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/access/postgres"
	"github.com/gabrielgcmr/sonnda/internal/features/patient/problem"
	problemhttp "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/http"
	problempostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/problem/postgres"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	profilehttp "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/http"
	patientpostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/postgres"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	patientcreationpostgres "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/patientcreation"
)

type PatientModule struct {
	Service           patientprofile.Service
	ProfileHandler    *profilehttp.Handler
	ProfileAuthorizer patientprofile.Authorizer
	ProblemAuthorizer problem.Authorizer
	ProblemHandler    *problemhttp.Handler
}

func NewPatientModule(db *postgress.Client) *PatientModule {
	patientRepo := patientpostgres.NewRepository(db)
	accessRepo := accesspostgres.NewRepository(db)

	accessChecker := patientaccess.NewChecker(patientRepo, accessRepo)
	profileAuthorizer := patientprofile.NewAuthorizer(accessChecker)
	svc := patientprofile.New(patientRepo, profileAuthorizer)
	creator := patientcreation.New(patientcreationpostgres.NewRepository(db))
	accounts := accountpostgres.New(db.Pool())
	problemAuthorizer := problem.NewAuthorizer(authz.NewPatientContextResolver(accounts, accessChecker))
	problemService := problem.New(problempostgres.NewRepository(db), problemAuthorizer)

	return &PatientModule{
		Service:           svc,
		ProfileHandler:    profilehttp.NewHandler(svc, creator),
		ProfileAuthorizer: profileAuthorizer,
		ProblemAuthorizer: problemAuthorizer,
		ProblemHandler:    problemhttp.NewHandler(problemService),
	}
}
