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
	ExtractPDF(context.Context, string, string) (*extraction.Result, error)
}

type StandaloneLabExtraction struct {
	extractor  StandaloneLabExtractor
	authorizer Authorizer
}

func NewStandaloneLabExtraction(extractor StandaloneLabExtractor, authorizer Authorizer) *StandaloneLabExtraction {
	return &StandaloneLabExtraction{extractor: extractor, authorizer: authorizer}
}

func (s *StandaloneLabExtraction) ExtractPDF(ctx context.Context, currentAccount *accountdomain.Account, path, filename string) (*extraction.Result, error) {
	if s == nil || s.extractor == nil || s.authorizer == nil {
		return nil, apperr.Internal("erro inesperado", errors.New("standalone lab extraction dependencies not configured"))
	}
	if err := s.authorizer.AuthorizeStandalone(currentAccount, ExtractStandaloneLab); err != nil {
		return nil, err
	}
	return s.extractor.ExtractPDF(ctx, path, filename)
}
