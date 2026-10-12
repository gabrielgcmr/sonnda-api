// internal/features/documentprocessing/http/temporary_document.go
package http

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"strings"

	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
)

type temporaryDocument struct {
	path     string
	mimeType string
}

func writeTemporaryDocument(header *multipart.FileHeader, maxFileSize int64) (*temporaryDocument, error) {
	if header == nil {
		return nil, apperr.Validation("arquivo é obrigatório", apperr.Violation{Field: "file", Reason: "required"})
	}
	if header.Size <= 0 {
		return nil, apperr.Validation("arquivo vazio", apperr.Violation{Field: "file", Reason: "empty"})
	}
	if header.Size > maxFileSize {
		return nil, documentSizeExceededError(maxFileSize)
	}

	source, err := header.Open()
	if err != nil {
		return nil, apperr.Internal("falha ao abrir arquivo", err)
	}
	defer source.Close()
	return copyTemporaryDocument(source, "", maxFileSize)
}

// writeTemporaryPDF preserves the PDF-only contract used by persisted patient documents.
func writeTemporaryPDF(header *multipart.FileHeader, maxFileSize int64) (string, error) {
	document, err := writeTemporaryDocument(header, maxFileSize)
	if err != nil {
		return "", err
	}
	if document.mimeType != "application/pdf" {
		_ = os.Remove(document.path)
		return "", apperr.Validation("Envie um arquivo PDF válido.")
	}
	return document.path, nil
}

func copyTemporaryDocument(source io.Reader, declaredMIME string, maxFileSize int64) (*temporaryDocument, error) {
	prefix := make([]byte, 8)
	count, err := io.ReadFull(source, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, apperr.Internal("falha ao ler arquivo", err)
	}
	prefix = prefix[:count]
	mimeType, extension := detectDocumentType(prefix)
	if mimeType == "" || (declaredMIME != "" && normalizeDocumentMIME(declaredMIME) != mimeType) {
		return nil, &apperr.AppError{Kind: apperr.UNSUPPORTED_MEDIA_TYPE, Message: "Envie um arquivo PDF, JPEG ou PNG válido."}
	}

	target, err := os.CreateTemp("", "sonnda-document-*"+extension)
	if err != nil {
		return nil, apperr.Internal("falha ao preparar arquivo temporário", err)
	}
	path := target.Name()
	combined := io.MultiReader(bytes.NewReader(prefix), source)
	written, copyErr := io.Copy(target, io.LimitReader(combined, maxFileSize+1))
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return nil, apperr.Internal("falha ao preparar arquivo temporário", errors.Join(copyErr, closeErr))
	}
	if written > maxFileSize {
		_ = os.Remove(path)
		return nil, documentSizeExceededError(maxFileSize)
	}
	return &temporaryDocument{path: path, mimeType: mimeType}, nil
}

func detectDocumentType(prefix []byte) (string, string) {
	switch {
	case len(prefix) >= 5 && bytes.Equal(prefix[:5], []byte("%PDF-")):
		return "application/pdf", ".pdf"
	case len(prefix) >= 3 && bytes.Equal(prefix[:3], []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg", ".jpg"
	case len(prefix) >= 8 && bytes.Equal(prefix[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png", ".png"
	default:
		return "", ""
	}
}

func normalizeDocumentMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "application/pdf", "application/x-pdf", "image/pdf":
		return "application/pdf"
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	default:
		return ""
	}
}

func documentSizeExceededError(maxFileSize int64) error {
	return &apperr.AppError{
		Kind:    apperr.UPLOAD_SIZE_EXCEEDED,
		Message: fmt.Sprintf("o arquivo deve ter no máximo %d MiB", maxFileSize/(1024*1024)),
	}
}
