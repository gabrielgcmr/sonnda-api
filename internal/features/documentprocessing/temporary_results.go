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
	ID        uuid.UUID       `json:"id"`
	AccountID uuid.UUID       `json:"-"`
	Kind      string          `json:"kind"`
	Result    json.RawMessage `json:"-"`
	Pages     []TemporaryPage `json:"pages"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiresAt time.Time       `json:"expires_at"`
}

type TemporaryResultRepository interface {
	Create(context.Context, *TemporaryResult) error
	List(context.Context, uuid.UUID, time.Time) ([]TemporaryResult, error)
	Get(context.Context, uuid.UUID, uuid.UUID, time.Time) (*TemporaryResult, error)
	Expired(context.Context, time.Time, int) ([]TemporaryResult, error)
	Delete(context.Context, uuid.UUID) error
}

type TemporaryFileStorage interface {
	Upload(context.Context, io.Reader, string, string) (string, error)
	Delete(context.Context, string) error
}

type TemporaryResults struct {
	repo    TemporaryResultRepository
	storage TemporaryFileStorage
	now     func() time.Time
}

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
	item := &TemporaryResult{ID: uuid.New(), AccountID: accountID, Kind: kind, Result: payload, Pages: make([]TemporaryPage, len(pages)), CreatedAt: s.now().UTC()}
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
