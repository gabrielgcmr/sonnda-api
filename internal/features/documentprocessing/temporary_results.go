// internal/features/documentprocessing/temporary_results.go
package documentprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gabrielgcmr/sonnda/internal/features/capture"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

const TemporaryRetention = 24 * time.Hour

var ErrTemporaryCaptureUsed = errors.New("capture already belongs to a temporary extraction")

type TemporaryPage struct {
	Position         int        `json:"position"`
	CaptureID        *uuid.UUID `json:"capture_id,omitempty"`
	StorageURI       string     `json:"-"`
	OriginalFilename string     `json:"original_filename"`
	MIMEType         string     `json:"mime_type"`
	SizeBytes        int64      `json:"size_bytes"`
	ExpiresAt        time.Time  `json:"expires_at"`
	LocalPath        string     `json:"-"`
}

type TemporaryResult struct {
	ID           uuid.UUID       `json:"id"`
	AccountID    uuid.UUID       `json:"-"`
	Kind         string          `json:"kind"`
	Result       json.RawMessage `json:"-"`
	Status       string          `json:"status"`
	Progress     int             `json:"progress"`
	ErrorMessage string          `json:"error_message,omitempty"`
	PatientName  string          `json:"patient_name,omitempty"`
	ExamName     string          `json:"exam_name,omitempty"`
	Pages        []TemporaryPage `json:"pages"`
	CreatedAt    time.Time       `json:"created_at"`
	ExpiresAt    time.Time       `json:"expires_at"`
}

type TemporaryResultRepository interface {
	Create(context.Context, *TemporaryResult) error
	List(context.Context, uuid.UUID, time.Time) ([]TemporaryResult, error)
	Get(context.Context, uuid.UUID, uuid.UUID, time.Time) (*TemporaryResult, error)
	Expired(context.Context, time.Time, int) ([]TemporaryResult, error)
	Delete(context.Context, uuid.UUID) error
	// Claim fails jobs whose expired lease already used maxAttempts before reserving the next one.
	Claim(ctx context.Context, owner uuid.UUID, now time.Time, lease time.Duration, maxAttempts int, exhaustedMessage string) (*TemporaryResult, error)
	Renew(context.Context, uuid.UUID, uuid.UUID, time.Time, time.Duration) (bool, error)
	Advance(context.Context, uuid.UUID, uuid.UUID, time.Time, int) (bool, error)
	Complete(context.Context, uuid.UUID, uuid.UUID, time.Time, json.RawMessage) (bool, error)
	Fail(context.Context, uuid.UUID, uuid.UUID, time.Time, string) (bool, error)
	// Release returns an interrupted job to the queue without consuming an attempt.
	Release(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	Retry(context.Context, uuid.UUID, uuid.UUID, time.Time) (bool, error)
}

type TemporaryFileStorage interface {
	Upload(context.Context, io.Reader, string, string) (string, error)
	Open(context.Context, string) (io.ReadCloser, error)
	GetSignedURL(context.Context, string, time.Duration) (string, error)
	Delete(context.Context, string) error
}

type temporaryCaptureSource interface {
	temporaryCaptureOpener
	GetCaptureFile(context.Context, uuid.UUID, uuid.UUID) (*capture.SignedCaptureFile, error)
}

const temporaryPageURLLifetime = 5 * time.Minute

type TemporaryPageFile struct {
	URL       string
	ExpiresAt time.Time
}

type TemporaryResults struct {
	repo     TemporaryResultRepository
	storage  TemporaryFileStorage
	captures temporaryCaptureSource
	now      func() time.Time
}

func (s *TemporaryResults) SetCaptureSource(source temporaryCaptureSource) { s.captures = source }

func NewTemporaryResults(repo TemporaryResultRepository, storage TemporaryFileStorage) *TemporaryResults {
	return &TemporaryResults{repo: repo, storage: storage, now: time.Now}
}

func (s *TemporaryResults) Save(ctx context.Context, accountID uuid.UUID, kind string, result any, pages []TemporaryPage) (saved *TemporaryResult, retErr error) {
	if s == nil || s.repo == nil || s.storage == nil {
		return nil, apperr.Internal("extrações temporárias indisponíveis", errors.New("temporary results dependencies missing"))
	}
	if accountID == uuid.Nil || (kind != "laboratory" && kind != "imaging") || len(pages) == 0 || result == nil {
		return nil, apperr.Validation("extração temporária inválida")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, apperr.Internal("não foi possível preparar o resultado", err)
	}
	return s.persist(ctx, accountID, kind, payload, "completed", pages)
}

func (s *TemporaryResults) Enqueue(ctx context.Context, accountID uuid.UUID, kind string, pages []TemporaryPage) (*TemporaryResult, error) {
	if s == nil || s.repo == nil || s.storage == nil {
		return nil, apperr.Internal("extrações temporárias indisponíveis", errors.New("temporary results dependencies missing"))
	}
	if accountID == uuid.Nil || (kind != "laboratory" && kind != "imaging") || len(pages) == 0 {
		return nil, apperr.Validation("extração temporária inválida")
	}
	return s.persist(ctx, accountID, kind, nil, "queued", pages)
}

func (s *TemporaryResults) persist(ctx context.Context, accountID uuid.UUID, kind string, payload json.RawMessage, status string, pages []TemporaryPage) (saved *TemporaryResult, retErr error) {
	var err error
	item := &TemporaryResult{ID: uuid.New(), AccountID: accountID, Kind: kind, Result: payload, Status: status, Pages: make([]TemporaryPage, len(pages)), CreatedAt: s.now().UTC()}
	if status == "completed" {
		item.Progress = 100
	}
	var uploaded []string
	committed := false
	defer func() {
		if committed || len(uploaded) == 0 {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		for _, uri := range uploaded {
			if cleanupErr := s.storage.Delete(cleanup, uri); cleanupErr != nil && !apperr.IsNotFound(cleanupErr) {
				retErr = apperr.Internal("falha ao limpar arquivo temporário", errors.Join(retErr, cleanupErr))
			}
		}
	}()
	for index, page := range pages {
		if page.SizeBytes <= 0 || page.SizeBytes > 10*1024*1024 || page.OriginalFilename == "" {
			err = apperr.Validation("página inválida")
			return nil, err
		}
		page.Position = index
		if page.CaptureID == nil {
			var file *os.File
			file, err = os.Open(page.LocalPath)
			if err != nil {
				return nil, apperr.Internal("não foi possível abrir o arquivo", err)
			}
			page.StorageURI, err = s.storage.Upload(ctx, file, fmt.Sprintf("accounts/%s/extractions/%s/%d", accountID, item.ID, index), page.MIMEType)
			_ = file.Close()
			if err != nil {
				return nil, apperr.Internal("não foi possível armazenar o arquivo", err)
			}
			uploaded = append(uploaded, page.StorageURI)
			page.ExpiresAt = s.now().UTC().Add(TemporaryRetention)
		}
		if !page.ExpiresAt.After(item.CreatedAt) {
			err = apperr.NotFound("captura expirada")
			return nil, err
		}
		if item.ExpiresAt.IsZero() || page.ExpiresAt.Before(item.ExpiresAt) {
			item.ExpiresAt = page.ExpiresAt
		}
		item.Pages[index] = page
	}
	err = s.repo.Create(ctx, item)
	if err != nil {
		if errors.Is(err, ErrTemporaryCaptureUsed) {
			return nil, apperr.Conflict("Esta captura já pertence a uma extração temporária.")
		}
		return nil, apperr.Internal("não foi possível salvar o resultado", err)
	}
	committed = true
	return item, nil
}

func (s *TemporaryResults) Retry(ctx context.Context, accountID, id uuid.UUID) (*TemporaryResult, error) {
	item, err := s.Get(ctx, accountID, id)
	if err != nil {
		return nil, err
	}
	if item.Status != "failed" {
		return nil, apperr.Conflict("A extração não está em falha.")
	}
	for _, page := range item.Pages {
		if !page.ExpiresAt.After(s.now().UTC()) {
			return nil, apperr.NotFound("arquivo expirado")
		}
		var body io.ReadCloser
		var err error
		if page.CaptureID != nil {
			if s.captures == nil {
				return nil, apperr.Internal("capturas indisponíveis", errors.New("capture source missing"))
			}
			file, openErr := s.captures.OpenCaptureFile(ctx, accountID, *page.CaptureID)
			if openErr != nil {
				return nil, apperr.NotFound("arquivo indisponível")
			}
			body = file.Body
		} else {
			body, err = s.storage.Open(ctx, page.StorageURI)
		}
		if err != nil {
			return nil, apperr.NotFound("arquivo indisponível")
		}
		_ = body.Close()
	}
	updated, err := s.repo.Retry(ctx, accountID, id, s.now().UTC())
	if err != nil {
		return nil, apperr.Internal("não foi possível tentar novamente", err)
	}
	if !updated {
		return nil, apperr.Conflict("A extração mudou de estado.")
	}
	item.Status, item.Progress, item.ErrorMessage = "queued", 0, ""
	return item, nil
}

// PageFile issues a short-lived URL only after Get confirms the account owns an available job.
func (s *TemporaryResults) PageFile(ctx context.Context, accountID, id uuid.UUID, position int) (*TemporaryPageFile, error) {
	item, err := s.Get(ctx, accountID, id)
	if err != nil {
		return nil, err
	}
	var page *TemporaryPage
	for index := range item.Pages {
		if item.Pages[index].Position == position {
			page = &item.Pages[index]
			break
		}
	}
	if page == nil {
		return nil, apperr.NotFound("página não encontrada")
	}
	if page.CaptureID != nil {
		if s.captures == nil {
			return nil, apperr.Internal("capturas indisponíveis", errors.New("capture source missing"))
		}
		file, err := s.captures.GetCaptureFile(ctx, accountID, *page.CaptureID)
		if err != nil {
			return nil, err
		}
		return &TemporaryPageFile{URL: file.URL, ExpiresAt: file.ExpiresAt}, nil
	}
	now := s.now().UTC()
	lifetime := min(temporaryPageURLLifetime, page.ExpiresAt.Sub(now)).Truncate(time.Second)
	if lifetime <= 0 {
		return nil, apperr.NotFound("arquivo expirado")
	}
	url, err := s.storage.GetSignedURL(ctx, page.StorageURI, lifetime)
	if err != nil {
		if apperr.IsNotFound(err) {
			return nil, apperr.NotFound("arquivo indisponível")
		}
		return nil, apperr.Internal("não foi possível acessar o arquivo", err)
	}
	return &TemporaryPageFile{URL: url, ExpiresAt: now.Add(lifetime)}, nil
}

func (s *TemporaryResults) List(ctx context.Context, accountID uuid.UUID) ([]TemporaryResult, error) {
	items, err := s.repo.List(ctx, accountID, s.now().UTC())
	if err != nil {
		return nil, apperr.Internal("não foi possível listar resultados", err)
	}
	return items, nil
}

func (s *TemporaryResults) Get(ctx context.Context, accountID, id uuid.UUID) (*TemporaryResult, error) {
	item, err := s.repo.Get(ctx, accountID, id, s.now().UTC())
	if err != nil {
		return nil, apperr.Internal("não foi possível consultar o resultado", err)
	}
	if item == nil {
		return nil, apperr.NotFound("resultado não encontrado")
	}
	return item, nil
}

// Cleanup is called by the existing scheduled capture cleanup job.
func (s *TemporaryResults) Cleanup(ctx context.Context) error {
	for {
		items, err := s.repo.Expired(ctx, s.now().UTC(), 50)
		if err != nil {
			return apperr.Internal("falha ao listar extrações expiradas", err)
		}
		if len(items) == 0 {
			return nil
		}
		for _, item := range items {
			for _, page := range item.Pages {
				if page.StorageURI != "" {
					if err := s.storage.Delete(ctx, page.StorageURI); err != nil && !apperr.IsNotFound(err) {
						return apperr.Internal("falha ao excluir arquivo temporário", err)
					}
				}
			}
			if err := s.repo.Delete(ctx, item.ID); err != nil {
				return apperr.Internal("falha ao excluir resultado temporário", err)
			}
		}
	}
}
