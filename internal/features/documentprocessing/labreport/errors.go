// internal/features/documentprocessing/labreport/errors.go
package labreport

import "errors"

var (
	ErrInvalidPatientID        = errors.New("patient id is required")
	ErrInvalidUploadedByUserID = errors.New("uploaded by user id is required")
	ErrInvalidStorageURI       = errors.New("storage uri is required")
	ErrInvalidOriginalFilename = errors.New("original filename is required")
	ErrInvalidMimeType         = errors.New("mime type is required")
	ErrInvalidStatus           = errors.New("invalid exam document status")
	ErrInvalidExamType         = errors.New("invalid exam type")
	ErrInvalidConfidence       = errors.New("confidence must be between 0 and 1")
	ErrInvalidExamDocumentText = errors.New("invalid exam document text")
	ErrInvalidText             = errors.New("text is required")
)
