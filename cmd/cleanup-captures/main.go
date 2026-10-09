// cmd/cleanup-captures/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/config"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	capturepostgres "github.com/gabrielgcmr/sonnda/internal/features/capture/postgres"
	postgress "github.com/gabrielgcmr/sonnda/internal/infrastructure/database/postgres"
	"github.com/gabrielgcmr/sonnda/internal/infrastructure/filestorage"
	"github.com/gabrielgcmr/sonnda/internal/kernel/observability"
)

func main() {
	batchSize := flag.Int("batch-size", capture.DefaultCleanupBatchSize, "Tamanho do lote para limpeza de capturas e sessões")
	uploadingTimeout := flag.Duration("uploading-timeout", capture.DefaultUploadingCutoffDuration, "Janela para considerar um upload como travado")
	timeout := flag.Duration("timeout", 10*time.Minute, "Tempo limite total para execução do job")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("falha ao carregar configuração: %v", err)
	}

	appLogger := observability.New(observability.Config{
		Env:       cfg.App.Env,
		Level:     cfg.App.LogLevel,
		Format:    cfg.App.LogFormat,
		AppName:   "github.com/gabrielgcmr/sonnda/cleanup-captures",
		AddSource: cfg.App.Env == "dev",
	})
	slog.SetDefault(appLogger)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	slog.Info("iniciando job de limpeza de capturas",
		slog.Int("batch_size", *batchSize),
		slog.Duration("uploading_timeout", *uploadingTimeout),
		slog.Duration("job_timeout", *timeout),
	)

	dbClient, err := postgress.NewClient(postgress.SupabaseConfig(cfg.Database.URL))
	if err != nil {
		slog.Error("falha ao conectar no banco de dados", "error", err)
		os.Exit(1)
	}
	defer dbClient.Close()

	supabaseStorageClient, err := filestorage.NewSupabaseStorageClient(filestorage.SupabaseClientConfig{
		ProjectURL: cfg.Auth.SupabaseProjectURL,
		SecretKey:  cfg.Storage.SupabaseSecretKey,
	})
	if err != nil {
		slog.Error("falha ao inicializar cliente do Supabase Storage", "error", err)
		os.Exit(1)
	}

	captureStorage, err := supabaseStorageClient.ForBucket(filestorage.BucketConfig{
		BucketName:  cfg.Storage.SupabaseCapturesBucket,
		MaxFileSize: 5 * 1024 * 1024,
	})
	if err != nil {
		slog.Error("falha ao configurar bucket de capturas", "error", err)
		os.Exit(1)
	}

	repository := capturepostgres.NewRepository(dbClient)
	service := capture.New(repository, captureStorage)

	startTime := time.Now()
	report, err := service.Cleanup(ctx, capture.CleanupOptions{
		BatchSize:             *batchSize,
		UploadingCutoffWindow: *uploadingTimeout,
	})

	duration := time.Since(startTime)

	if err != nil {
		slog.Error("falha fatal na execução do job de limpeza",
			"error", err,
			"duration", duration,
		)
		os.Exit(1)
	}

	if len(report.Errors) > 0 {
		for _, warnErr := range report.Errors {
			slog.Warn("aviso durante limpeza de captura", "error", warnErr)
		}
	}

	slog.Info("job de limpeza de capturas concluído com sucesso",
		slog.Int("captures_processed", report.CapturesProcessed),
		slog.Int("captures_deleted", report.CapturesDeleted),
		slog.Int("storage_deleted", report.StorageDeleted),
		slog.Int("sessions_deleted", report.SessionsDeleted),
		slog.Int("warning_count", len(report.Errors)),
		slog.Duration("duration", duration),
	)

	fmt.Printf("Limpeza concluída em %v: %d capturas processadas, %d excluídas, %d no storage, %d sessões removidas\n",
		duration, report.CapturesProcessed, report.CapturesDeleted, report.StorageDeleted, report.SessionsDeleted)
}

