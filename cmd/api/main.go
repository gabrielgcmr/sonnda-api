// cmd/api/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/gabrielgcmr/sonnda/internal/application/bootstrap"
	"github.com/gabrielgcmr/sonnda/internal/config"
	authhttp "github.com/gabrielgcmr/sonnda/internal/features/auth/http"
	capturedomain "github.com/gabrielgcmr/sonnda/internal/features/capture/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
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
	//6.1 Storage Service (Supabase)
	supabaseStorageClient, err := filestorage.NewSupabaseStorageClient(filestorage.SupabaseClientConfig{
		ProjectURL: cfg.Auth.SupabaseProjectURL,
		SecretKey:  cfg.Storage.SupabaseSecretKey,
	})
	if err != nil {
		logInfraFatal("falha ao criar cliente do Supabase Storage", err)
	}
	examDocumentsStorage, err := supabaseStorageClient.ForBucket(filestorage.BucketConfig{
		BucketName:  cfg.Storage.SupabaseExamDocumentsBucket,
		MaxFileSize: documentprocessing.MaxFileSizeBytes,
	})
	if err != nil {
		logInfraFatal("falha ao configurar bucket de documentos de exames", err)
	}
	captureStorage, err := supabaseStorageClient.ForBucket(filestorage.BucketConfig{
		BucketName:  cfg.Storage.SupabaseCapturesBucket,
		MaxFileSize: capturedomain.MaxFileSizeBytes,
	})
	if err != nil {
		logInfraFatal("falha ao configurar bucket de capturas", err)
	}
	temporaryStorage, err := supabaseStorageClient.ForBucket(filestorage.BucketConfig{
		BucketName: cfg.Storage.SupabaseTemporaryBucket, MaxFileSize: 10 * 1024 * 1024,
	})
	if err != nil {
		logInfraFatal("falha ao configurar bucket de extrações temporárias", err)
	}

	var labExtractor labreport.LabReportExtractor
	var imagingExtractor imagingreport.Extractor
	if strings.TrimSpace(cfg.Gemini.APIKey) != "" {
		geminiClient, err := geminiinfra.NewClient(ctx, cfg.Gemini)
		if err != nil {
			logInfraFatal("falha ao criar Gemini client", err)
		}
		labExtractor, err = geminiinfra.NewLabReportExtractor(geminiClient)
		if err != nil {
			logInfraFatal("falha ao criar extrator laboratorial", err)
		}
		imagingExtractor, err = geminiinfra.NewImagingReportExtractor(geminiClient)
		if err != nil {
			logInfraFatal("falha ao criar extrator de laudos de imagem", err)
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
	modules := bootstrap.NewModules(dbClient, redisClient, labExtractor, imagingExtractor, examDocumentsStorage, captureStorage, temporaryStorage, cfg.OCR, cfg.ProfessionalActivation, cfg.Jobs.CaptureCleanupToken)

	//8 Middlewares
	//8.1 API
	authMiddleware := authhttp.NewMiddleware(apiAuthProvider.AuthenticateBearerToken)

	//10. Cria o router HTTP
	gin.SetMode(gin.ReleaseMode)

	app, err := api.New(api.Options{
		Name:       "Sonnda API",
		Version:    version,
		Logger:     appLogger,
		CORSConfig: cfg.CORS,
		Deps: &api.APIDependencies{
			Auth:                               authMiddleware,
			Account:                            modules.Account.Middleware,
			AccountHandler:                     modules.Account.Handler,
			CaptureHandler:                     modules.Capture.Handler,
			CaptureAuth:                        modules.Capture.Middleware,
			CaptureCleanup:                     modules.Capture.Cleanup,
			PatientAccessHandler:               modules.PatientAccess.Handler,
			PatientHandler:                     modules.Patient.ProfileHandler,
			PatientProblemHandler:              modules.Patient.ProblemHandler,
			LaboratoryHandler:                  modules.Labs.LaboratoryHandler,
			ExamsHandler:                       modules.Exams.Handler,
			StandaloneLabExtractionHandler:     modules.Exams.StandaloneLabExtractionHandler,
			StandaloneImagingExtractionHandler: modules.Exams.StandaloneImagingExtractionHandler,
			TemporaryExtractionHandler:         modules.Exams.TemporaryExtractionHandler,
		},
	})
	if err != nil {
		logInfraFatal("falha ao configurar API", err)
	}
	go modules.Exams.TemporaryWorker.Run(ctx)

	// 10. Inicia o servidor
	fmt.Printf(
		"\n────────────────────────────────────────\n SONNDA IS RUNNING\n────────────────────────────────────────\n  mode          : %s\n  LOCAL_API_URL : http://localhost:%s\n  PUBLIC_API_URL: https://api.sonnda.com.br\n\n",
		cfg.App.Env,
		cfg.HTTP.Port,
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
		slog.Error(prefix)
		os.Exit(1)
	}

	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr != nil {
		if appErr.Cause != nil {
			slog.Error(prefix, "message", appErr.Message, "cause", appErr.Cause)
		} else {
			slog.Error(prefix, "message", appErr.Message)
		}
		os.Exit(1)
	}

	slog.Error(prefix, "error", err)
	os.Exit(1)
}
