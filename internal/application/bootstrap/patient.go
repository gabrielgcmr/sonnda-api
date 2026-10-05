// internal/application/bootstrap/patient.go
package bootstrap

import (
	patientcreation "github.com/gabrielgcmr/sonnda/internal/application/usecase/patientcreation"
	accountpostgres "github.com/gabrielgcmr/sonnda/internal/features/account/postgres"
	"github.com/gabrielgcmr/sonnda/internal/features/authz"
	patientaccess "github.com/gabrielgcmr/sonnda/internal/features/patient/access"
	accesspostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/access/postgres"
	patienthttp "github.com/gabrielgcmr/sonnda/internal/features/patient/http"
	patientprofile "github.com/gabrielgcmr/sonnda/internal/features/patient/profile"
	profilehttp "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/http"
	patientpostgres "github.com/gabrielgcmr/sonnda/internal/features/patient/profile/postgres"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	patientcreationpostgres "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres/patientcreation"
)

type PatientModule struct {
	Service           patientprofile.Service
	ProfileHandler    *profilehttp.Handler
	CreationHandler   *patienthttp.CreationHandler
	ProblemAuthorizer authz.ProblemAuthorizer
}

func NewPatientModule(db *postgress.Client) *PatientModule {
	patientRepo := patientpostgres.NewRepository(db)
	accessRepo := accesspostgres.NewRepository(db)

	accessChecker := patientaccess.NewChecker(patientRepo, accessRepo)
	svc := patientprofile.New(patientRepo, accessRepo, accessChecker)
	creator := patientcreation.New(patientcreationpostgres.NewRepository(db))
	accounts := accountpostgres.New(db.Pool())

	return &PatientModule{
		Service:         svc,
		ProfileHandler:  profilehttp.NewHandler(svc),
		CreationHandler: patienthttp.NewCreationHandler(creator),
		ProblemAuthorizer: authz.NewProblemAuthorizer(
			authz.NewPatientContextResolver(accounts, accessChecker),
		),
	}
}
