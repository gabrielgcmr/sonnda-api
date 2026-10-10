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

const (
	exitSuccess = 0
	exitFailure = 1
)

func main() {
	os.Exit(run())
}

func run() int {
	batchSize := flag.Int("batch-size", capture.DefaultCleanupBatchSize, "Tamanho do lote para limpeza de capturas e sessões")
	uploadingTimeout := flag.Duration("uploading-timeout", capture.DefaultUploadingCutoffDuration, "Janela para considerar um upload como travado")
	timeout := flag.Duration("timeout", 10*time.Minute, "Tempo limite total para execução do job")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Printf("falha ao carregar configuração: %v", err)
		return exitFailure
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
		return exitFailure
	}
	defer dbClient.Close()

	supabaseStorageClient, err := filestorage.NewSupabaseStorageClient(filestorage.SupabaseClientConfig{
		ProjectURL: cfg.Auth.SupabaseProjectURL,
		SecretKey:  cfg.Storage.SupabaseSecretKey,
	})
	if err != nil {
		slog.Error("falha ao inicializar cliente do Supabase Storage", "error", err)
		return exitFailure
	}

	captureStorage, err := supabaseStorageClient.ForBucket(filestorage.BucketConfig{
		BucketName:  cfg.Storage.SupabaseCapturesBucket,
		MaxFileSize: 5 * 1024 * 1024,
	})
	if err != nil {
		slog.Error("falha ao configurar bucket de capturas", "error", err)
		return exitFailure
	}

	repository := capturepostgres.NewRepository(dbClient)
	service := capture.New(repository, captureStorage)

	startTime := time.Now()
	report, err := service.Cleanup(ctx, capture.CleanupOptions{
		BatchSize:             *batchSize,
		UploadingCutoffWindow: *uploadingTimeout,
	})

	duration := time.Since(startTime)
	exitCode := cleanupExitCode(report, err)

	if err != nil {
		slog.Error("falha fatal na execução do job de limpeza",
			"error", err,
			"run_id", cleanupRunID(report),
			"duration", duration,
		)
		return exitCode
	}

	if report == nil {
		slog.Error("job de limpeza não retornou relatório", "duration", duration)
		return exitCode
	}

	if exitCode != exitSuccess {
		for _, cleanupErr := range report.Errors {
			slog.Error("falha durante limpeza de captura", "error", cleanupErr)
		}
		slog.Error("job de limpeza de capturas concluído com falhas",
			slog.String("run_id", report.RunID.String()),
			slog.Int("captures_processed", report.CapturesProcessed),
			slog.Int("captures_deleted", report.CapturesDeleted),
			slog.Int("storage_deleted", report.StorageDeleted),
			slog.Int("sessions_deleted", report.SessionsDeleted),
			slog.Int("error_count", len(report.Errors)),
			slog.Duration("duration", duration),
		)
		fmt.Fprintf(os.Stderr, "Limpeza incompleta em %v: %d erro(s)\n", duration, len(report.Errors))
		return exitCode
	}

	slog.Info("job de limpeza de capturas concluído com sucesso",
		slog.String("run_id", report.RunID.String()),
		slog.Int("captures_processed", report.CapturesProcessed),
		slog.Int("captures_deleted", report.CapturesDeleted),
		slog.Int("storage_deleted", report.StorageDeleted),
		slog.Int("sessions_deleted", report.SessionsDeleted),
		slog.Int("warning_count", len(report.Errors)),
		slog.Duration("duration", duration),
	)

	fmt.Printf("Limpeza concluída em %v: %d capturas processadas, %d excluídas, %d no storage, %d sessões removidas\n",
		duration, report.CapturesProcessed, report.CapturesDeleted, report.StorageDeleted, report.SessionsDeleted)
	return exitSuccess
}

func cleanupExitCode(report *capture.CleanupReport, err error) int {
	if err != nil || report == nil || len(report.Errors) > 0 {
		return exitFailure
	}
	return exitSuccess
}

func cleanupRunID(report *capture.CleanupReport) string {
	if report == nil {
		return ""
	}
	return report.RunID.String()
}
