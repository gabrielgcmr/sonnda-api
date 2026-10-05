// cmd/api/main.go
package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
	"google.golang.org/api/option"

	"github.com/gabrielgcmr/sonnda/internal/application/bootstrap"
	"github.com/gabrielgcmr/sonnda/internal/config"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labextraction"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/gabrielgcmr/sonnda/internal/kernel/observability"

	"github.com/gabrielgcmr/sonnda/internal/api"
	authinfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/auth"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	filestorage "github.com/gabrielgcmr/sonnda/internal/infrastructure/filestorage"
	geminiinfra "github.com/gabrielgcmr/sonnda/internal/infrastructure/gemini"
	redisstore "github.com/gabrielgcmr/sonnda/internal/infrastructure/redis"
	"github.com/redis/go-redis/v9"
)

// version is overridden via -ldflags in build/release pipelines.
var version = "dev"

func main() {
	// 1. Carrega o contexto
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Carrega configuracao
	cfg, err := config.Load()
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) && appErr != nil && len(appErr.Violations) > 0 {
			log.Printf("Erro ao carregar configuracao: %s", appErr.Message)
			for _, v := range appErr.Violations {
				log.Printf(" - %s: %s", v.Field, v.Reason)
			}
			log.Fatal("configuração inválida")
		}
		log.Fatal("Erro ao carregar configuracao: ", err)
	}

	// 3. Carrega logger
	appLogger := observability.New(observability.Config{
		Env:       cfg.App.Env,
		Level:     cfg.App.LogLevel,
		Format:    cfg.App.LogFormat,
		AppName:   "github.com/gabrielgcmr/sonnda",
		AddSource: cfg.App.Env == "dev",
	})
	slog.SetDefault(appLogger)

	// 4. Persistence
	// 4.1 Conectar db (Supabase via pgxpool)
	dbClient, err := postgress.NewClient(postgress.SupabaseConfig(cfg.Database.URL))
	if err != nil {
		log.Fatalf("falha ao criar client do supabase: %v", err)
	}
	defer dbClient.Close()

	var redisClient *redis.Client
	if cfg.Database.RedisURL != "" {
		redisClient, err = redisstore.NewClient(cfg.Database.RedisURL)
		if err != nil {
			logInfraFatal("falha ao criar client do Redis", err)
		}
		defer redisClient.Close()
	}

	//6. Conectando outros servicos
	//6.1 Storage Service (GCS)
	gcpOpts := buildGCPClientOptions(cfg)
	storageService, err := filestorage.NewGCSObjectStorage(ctx, cfg.Storage.GCSBucket, cfg.Storage.GCPProjectID, gcpOpts...)
	if err != nil {
		logInfraFatal("falha ao criar storage do GCS", err)
	}
	defer storageService.Close()

	var labTextExtractor labextraction.LabReportTextExtractor
	if strings.TrimSpace(cfg.Gemini.APIKey) != "" {
		geminiClient, err := geminiinfra.NewClient(ctx, cfg.Gemini)
		if err != nil {
			logInfraFatal("falha ao criar Gemini client", err)
		}
		labTextExtractor, err = geminiinfra.NewLabReportTextExtractor(geminiClient)
		if err != nil {
			logInfraFatal("falha ao criar extrator laboratorial", err)
		}
	}

	//6.3 Auth (Supabase)
	apiAuthProvider, err := authinfra.NewSupabaseBearerProvider(authinfra.SupabaseBearerConfig{
		SupabaseURL: cfg.Auth.SupabaseProjectURL,
		Issuer:      cfg.Auth.SupabaseJWTIssuer,
		Audience:    cfg.Auth.SupabaseJWTAudience,
	})
	if err != nil {
		logInfraFatal("falha ao criar supabase bearer provider", err)
	}

	//7. Módulos
	modules := bootstrap.NewModules(dbClient, redisClient, labTextExtractor, storageService, cfg.OCR, cfg.ProfessionalActivation)

	//8 Middlewares
	//8.1 API
	authMiddleware := authhttp.NewMiddleware(apiAuthProvider.AuthenticateBearerToken)

	//10. Cria o router HTTP
	ginMode := gin.DebugMode
	if cfg.App.Env == "prod" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)

	app := api.New(api.Options{
		Name:       "Sonnda API",
		Version:    version,
		Logger:     appLogger,
		CORSConfig: cfg.CORS,
		Deps: &api.APIDependencies{
			Auth:                           authMiddleware,
			Account:                        modules.Account.Middleware,
			AccountHandler:                 modules.Account.Handler,
			PatientAccessHandler:           modules.PatientAccess.Handler,
			PatientCreationHandler:         modules.Patient.CreationHandler,
			PatientHandler:                 modules.Patient.ProfileHandler,
			PatientProblemHandler:          modules.Patient.ProblemHandler,
			LaboratoryHandler:              modules.Labs.LaboratoryHandler,
			ExamsHandler:                   modules.Exams.Handler,
			StandaloneLabExtractionHandler: modules.Exams.StandaloneLabExtractionHandler,
		},
	})

	// 10. Inicia o servidor
	slog.Info(
		"Sonnda is running",
		slog.String("mode", cfg.App.Env),
		slog.String("local_api_url", "http://localhost:"+cfg.HTTP.Port),
		slog.String("public_api_url", "https://api.sonnda.com.br"),
	)
	if err := app.Run(ctx, ":"+cfg.HTTP.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// 1. Loga o erro com nivel Error (estruturado)
		slog.Error("failed to start server", "error", err)

		// 2. Encerra o programa manualmente com codigo de erro 1
		os.Exit(1)
	}
}

func logInfraFatal(prefix string, err error) {
	if err == nil {
		log.Fatal(prefix)
	}

	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr != nil {
		if appErr.Cause != nil {
			log.Fatalf("%s: %s (cause: %v)", prefix, appErr.Message, appErr.Cause)
		}
		log.Fatalf("%s: %s", prefix, appErr.Message)
	}

	log.Fatalf("%s: %v", prefix, err)
}

func buildGCPClientOptions(cfg *config.Config) []option.ClientOption {
	if cfg == nil {
		return nil
	}
	if cfg.Storage.GoogleApplicationCredentialsJSON != "" {
		return []option.ClientOption{option.WithCredentialsJSON([]byte(cfg.Storage.GoogleApplicationCredentialsJSON))}
	}
	if cfg.Storage.GoogleApplicationCredentials != "" {
		return []option.ClientOption{option.WithCredentialsFile(cfg.Storage.GoogleApplicationCredentials)}
	}
	return nil
}
