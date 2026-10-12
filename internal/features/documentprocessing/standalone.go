// internal/features/documentprocessing/standalone.go
package documentprocessing

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/extraction"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type StandaloneLabExtractor interface {
	ExtractDocument(context.Context, string, string, string) (*extraction.Result, error)
}

type StandaloneLabExtraction struct {
	extractor  StandaloneLabExtractor
	authorizer Authorizer
}

func NewStandaloneLabExtraction(extractor StandaloneLabExtractor, authorizer Authorizer) *StandaloneLabExtraction {
	return &StandaloneLabExtraction{extractor: extractor, authorizer: authorizer}
}

func (s *StandaloneLabExtraction) Authorize(currentAccount *accountdomain.Account) error {
	if s == nil || s.extractor == nil || s.authorizer == nil {
		return apperr.Internal("erro inesperado", errors.New("standalone lab extraction dependencies not configured"))
	}
	return s.authorizer.AuthorizeStandalone(currentAccount, ExtractStandaloneLab)
}

func (s *StandaloneLabExtraction) ExtractDocument(ctx context.Context, currentAccount *accountdomain.Account, path, filename, mimeType string) (*extraction.Result, error) {
	if err := s.Authorize(currentAccount); err != nil {
		return nil, err
	}
	return s.extractor.ExtractDocument(ctx, path, filename, mimeType)
}
