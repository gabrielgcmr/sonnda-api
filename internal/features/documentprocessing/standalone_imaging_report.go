// internal/features/documentprocessing/standalone_imaging_report.go
package documentprocessing

import (
	"context"
	"errors"

	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	"github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/imagingreport"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type StandaloneImagingExtractor interface {
	ExtractDocument(context.Context, string, string, string) (*imagingreport.ImagingReportResult, error)
	ExtractImagePages(context.Context, []imagingreport.DocumentPage) (*imagingreport.ImagingReportResult, error)
}

type StandaloneImagingExtraction struct {
	extractor  StandaloneImagingExtractor
	authorizer Authorizer
}

func NewStandaloneImagingExtraction(extractor StandaloneImagingExtractor, authorizer Authorizer) *StandaloneImagingExtraction {
	return &StandaloneImagingExtraction{extractor: extractor, authorizer: authorizer}
}

func (s *StandaloneImagingExtraction) Authorize(currentAccount *accountdomain.Account) error {
	if s == nil || s.extractor == nil || s.authorizer == nil {
		return apperr.Internal("erro inesperado", errors.New("standalone imaging extraction dependencies not configured"))
	}
	return s.authorizer.AuthorizeStandalone(currentAccount, ExtractStandaloneImaging)
}

func (s *StandaloneImagingExtraction) ExtractDocument(ctx context.Context, account *accountdomain.Account, path, filename, mimeType string) (*imagingreport.ImagingReportResult, error) {
	if err := s.Authorize(account); err != nil {
		return nil, err
	}
	return s.extractor.ExtractDocument(ctx, path, filename, mimeType)
}

func (s *StandaloneImagingExtraction) ExtractImagePages(ctx context.Context, account *accountdomain.Account, pages []imagingreport.DocumentPage) (*imagingreport.ImagingReportResult, error) {
	if err := s.Authorize(account); err != nil {
		return nil, err
	}
	return s.extractor.ExtractImagePages(ctx, pages)
}
