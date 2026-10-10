// internal/features/patient/problem/domain/errors.go
package problemdomain

import "errors"

var (
	ErrInvalidID                   = errors.New("invalid problem id")
	ErrInvalidPatientID            = errors.New("invalid patient id")
	ErrInvalidActorID              = errors.New("invalid actor account id")
	ErrInvalidName                 = errors.New("problem name is required")
	ErrInvalidCID11                = errors.New("CID-11 code is required when coding is provided")
	ErrInvalidClassification       = errors.New("invalid problem classification")
	ErrInvalidClinicalStatus       = errors.New("invalid clinical status")
	ErrInvalidAdministrativeStatus = errors.New("invalid administrative status")
	ErrInvalidMergeTarget          = errors.New("invalid merge target")
	ErrInvalidTimestamp            = errors.New("invalid timestamp")
	ErrInvalidVersion              = errors.New("invalid version")
	ErrChronicResolved             = errors.New("chronic problem cannot be resolved")
	ErrInvalidTransition           = errors.New("invalid problem transition")
	ErrReasonRequired              = errors.New("reason is required")
	ErrInvalidMergeSources         = errors.New("invalid merge sources")
)
