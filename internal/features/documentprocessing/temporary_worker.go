// internal/features/documentprocessing/temporary_worker.go
package documentprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/labreport"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

const (
	temporaryLease         = 2 * time.Minute
	temporaryMaxAttempts   = 3
	temporaryLabBatchBytes = 10 * 1024 * 1024
)

const (
	temporaryFailureMessage   = "Não foi possível processar o arquivo. Tente novamente."
	temporaryExhaustedMessage = "O processamento foi interrompido repetidas vezes. Tente novamente."
	temporaryExpiredMessage   = "Os arquivos deste trabalho expiraram ou foram removidos."
)

type temporaryAccountFinder interface {
	FindByID(context.Context, uuid.UUID) (*accountdomain.Account, error)
}

type temporaryCaptureOpener interface {
	OpenCaptureFile(context.Context, uuid.UUID, uuid.UUID) (*capture.CaptureFile, error)
}

type temporaryLabExtractor interface {
	ExtractDocument(context.Context, *accountdomain.Account, string, string, string) (*labreport.Result, error)
}

type temporaryImagingExtractor interface {
	ExtractDocument(context.Context, *accountdomain.Account, string, string, string) (*imagingreport.ImagingReportResult, error)
	ExtractImagePages(context.Context, *accountdomain.Account, []imagingreport.DocumentPage) (*imagingreport.ImagingReportResult, error)
}

type TemporaryWorker struct {
	results  *TemporaryResults
	accounts temporaryAccountFinder
	captures temporaryCaptureOpener
	lab      temporaryLabExtractor
	imaging  temporaryImagingExtractor
	owner    uuid.UUID
}

func NewTemporaryWorker(results *TemporaryResults, accounts temporaryAccountFinder, captures temporaryCaptureOpener, lab temporaryLabExtractor, imaging temporaryImagingExtractor) *TemporaryWorker {
	return &TemporaryWorker{results: results, accounts: accounts, captures: captures, lab: lab, imaging: imaging, owner: uuid.New()}
}

// Run polls the durable queue. The database lease makes multiple API instances safe.
func (w *TemporaryWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		processed, err := w.ProcessOne(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("temporary extraction worker failed", "error", err)
		}
		if processed && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *TemporaryWorker) ProcessOne(ctx context.Context) (bool, error) {
	item, err := w.results.repo.Claim(ctx, w.owner, w.results.now().UTC(), temporaryLease, temporaryMaxAttempts, temporaryExhaustedMessage)
	if err != nil || item == nil {
		return false, err
	}
	jobCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	renewDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				renewDone <- nil
				return
			case <-ticker.C:
				ok, err := w.results.repo.Renew(jobCtx, item.ID, w.owner, w.results.now().UTC(), temporaryLease)
				if err != nil || !ok {
					cancel()
					if err == nil {
						err = errors.New("lease no longer owned")
					}
					renewDone <- fmt.Errorf("temporary extraction lease lost: %w", err)
					return
				}
			}
		}
	}()
	result, processErr := w.safeExtract(jobCtx, item)
	cancel()
	leaseErr := <-renewDone
	if ctx.Err() != nil {
		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer releaseCancel()
		if _, err := w.results.repo.Release(releaseCtx, item.ID, w.owner); err != nil {
			return true, errors.Join(ctx.Err(), err)
		}
		return true, ctx.Err()
	}
	if leaseErr != nil {
		return true, leaseErr
	}
	if processErr != nil {
		message := temporaryFailureMessage
		var appErr *apperr.AppError
		if errors.As(processErr, &appErr) {
			message = appErr.Message
		}
		if err := w.fail(ctx, item.ID, message); err != nil {
			return true, errors.Join(processErr, err)
		}
		slog.Warn("temporary extraction failed", "id", item.ID, "error", processErr)
		return true, nil
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return true, errors.Join(err, w.fail(ctx, item.ID, temporaryFailureMessage))
	}
	ok, err := w.results.repo.Complete(ctx, item.ID, w.owner, w.results.now().UTC(), payload)
	if err != nil {
		return true, err
	}
	if !ok {
		// Complete refuses expired or removed sources; record that instead of waiting for the lease to lapse.
		if err := w.fail(ctx, item.ID, temporaryExpiredMessage); err != nil {
			return true, err
		}
		slog.Warn("temporary extraction sources expired before completion", "id", item.ID)
	}
	return true, nil
}

func (w *TemporaryWorker) fail(ctx context.Context, id uuid.UUID, message string) error {
	ok, err := w.results.repo.Fail(ctx, id, w.owner, w.results.now().UTC(), message)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("temporary extraction expired or lost its lease before recording failure")
	}
	return nil
}

func (w *TemporaryWorker) safeExtract(ctx context.Context, item *TemporaryResult) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = apperr.Internal(temporaryFailureMessage, fmt.Errorf("temporary extraction panic: %v", recovered))
		}
	}()
	return w.extract(ctx, item)
}

func (w *TemporaryWorker) extract(ctx context.Context, item *TemporaryResult) (any, error) {
	account, err := w.accounts.FindByID(ctx, item.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.DeletedAt != nil {
		return nil, apperr.NotFound("conta indisponível")
	}
	if len(item.Pages) == 0 {
		return nil, apperr.NotFound("arquivo indisponível")
	}
	local := make([]TemporaryPage, 0, len(item.Pages))
	defer func() {
		for _, page := range local {
			_ = os.Remove(page.LocalPath)
		}
	}()
	for _, page := range item.Pages {
		if !page.ExpiresAt.After(w.results.now().UTC()) {
			return nil, apperr.NotFound("arquivo expirado")
		}
		var source io.ReadCloser
		if page.CaptureID != nil {
			file, err := w.captures.OpenCaptureFile(ctx, item.AccountID, *page.CaptureID)
			if err != nil {
				return nil, err
			}
			source = file.Body
		} else {
			source, err = w.results.storage.Open(ctx, page.StorageURI)
			if err != nil {
				return nil, err
			}
		}
		file, err := os.CreateTemp("", "sonnda-temporary-job-*")
		if err != nil {
			_ = source.Close()
			return nil, err
		}
		page.LocalPath = file.Name()
		local = append(local, page)
		count, copyErr := io.Copy(file, io.LimitReader(source, page.SizeBytes+1))
		closeErr := errors.Join(file.Close(), source.Close())
		if copyErr != nil || closeErr != nil {
			return nil, errors.Join(copyErr, closeErr)
		}
		if count != page.SizeBytes {
			return nil, apperr.NotFound("arquivo indisponível")
		}
	}
	if err := w.advance(ctx, item.ID, 30); err != nil {
		return nil, err
	}
	var result any
	if item.Kind == "laboratory" {
		if len(local) == 1 {
			page := local[0]
			result, err = w.lab.ExtractDocument(ctx, account, page.LocalPath, page.OriginalFilename, page.MIMEType)
		} else {
			pageResults := make([]*labreport.Result, 0, len(local))
			for start := 0; start < len(local); {
				end := start
				var bytesInBatch int64
				for end < len(local) && end-start < 4 && bytesInBatch+local[end].SizeBytes <= temporaryLabBatchBytes {
					bytesInBatch += local[end].SizeBytes
					end++
				}
				if end == start {
					return nil, apperr.Validation("imagem acima do limite de 10 MiB")
				}
				for _, page := range local[start:end] {
					pageResult, pageErr := w.lab.ExtractDocument(ctx, account, page.LocalPath, page.OriginalFilename, page.MIMEType)
					if pageErr != nil {
						return nil, pageErr
					}
					if pageResult == nil {
						return nil, apperr.Internal("resultado da extração indisponível", errors.New("lab extractor returned nil"))
					}
					pageResults = append(pageResults, pageResult)
				}
				if err := w.advance(ctx, item.ID, 30+int(55*end/len(local))); err != nil {
					return nil, err
				}
				start = end
			}
			result = labreport.MergePages(pageResults)
		}
	} else if len(local) == 1 {
		page := local[0]
		result, err = w.imaging.ExtractDocument(ctx, account, page.LocalPath, page.OriginalFilename, page.MIMEType)
	} else {
		pages := make([]imagingreport.DocumentPage, len(local))
		for i, page := range local {
			pages[i] = imagingreport.DocumentPage{Path: page.LocalPath, Filename: page.OriginalFilename, MIMEType: page.MIMEType}
		}
		result, err = w.imaging.ExtractImagePages(ctx, account, pages)
	}
	if err != nil {
		return nil, err
	}
	switch value := result.(type) {
	case *labreport.Result:
		if value == nil {
			return nil, apperr.Internal("resultado da extração indisponível", errors.New("lab extractor returned nil"))
		}
	case *imagingreport.ImagingReportResult:
		if value == nil {
			return nil, apperr.Internal("resultado da extração indisponível", errors.New("imaging extractor returned nil"))
		}
	}
	if err := w.advance(ctx, item.ID, 90); err != nil {
		return nil, err
	}
	// Storage objects can disappear while extraction runs; check all sources again.
	for _, page := range item.Pages {
		if !page.ExpiresAt.After(w.results.now().UTC()) {
			return nil, apperr.NotFound("arquivo expirado")
		}
		var body io.ReadCloser
		if page.CaptureID != nil {
			file, openErr := w.captures.OpenCaptureFile(ctx, item.AccountID, *page.CaptureID)
			if openErr != nil {
				return nil, openErr
			}
			body = file.Body
		} else {
			body, err = w.results.storage.Open(ctx, page.StorageURI)
			if err != nil {
				return nil, err
			}
		}
		_ = body.Close()
	}
	return result, nil
}

func (w *TemporaryWorker) advance(ctx context.Context, id uuid.UUID, progress int) error {
	ok, err := w.results.repo.Advance(ctx, id, w.owner, w.results.now().UTC(), progress)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("temporary extraction lease lost")
	}
	return nil
}
