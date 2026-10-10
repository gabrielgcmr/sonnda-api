// internal/features/patient/problem/domain/types.go
package problemdomain

import (
	"strings"

	"github.com/google/uuid"
)

type Classification string

const (
	ClassificationAcute   Classification = "acute"
	ClassificationChronic Classification = "chronic"
)

func (c Classification) IsValid() bool {
	return c == ClassificationAcute || c == ClassificationChronic
}

type ClinicalStatus string

const (
	ClinicalStatusActive   ClinicalStatus = "active"
	ClinicalStatusResolved ClinicalStatus = "resolved"
)

func (s ClinicalStatus) IsValid() bool {
	return s == ClinicalStatusActive || s == ClinicalStatusResolved
}

type AdministrativeStatus string

const (
	AdministrativeStatusValid          AdministrativeStatus = "valid"
	AdministrativeStatusMerged         AdministrativeStatus = "merged"
	AdministrativeStatusEnteredInError AdministrativeStatus = "entered_in_error"
)

func (s AdministrativeStatus) IsValid() bool {
	return s == AdministrativeStatusValid || s == AdministrativeStatusMerged || s == AdministrativeStatusEnteredInError
}

// CID11 preserves the code and its coding-system metadata as supplied.
type CID11 struct {
	Code    string
	System  string
	Version string
}

func (c CID11) normalized() CID11 {
	c.Code = strings.TrimSpace(c.Code)
	c.System = strings.TrimSpace(c.System)
	c.Version = strings.TrimSpace(c.Version)
	return c
}

// Snapshot contains the fields whose before/after values must be audited.
type Snapshot struct {
	Name                 string
	CID11                *CID11
	Classification       Classification
	ClinicalStatus       ClinicalStatus
	AdministrativeStatus AdministrativeStatus
	MergedIntoID         *uuid.UUID
}

func (s Snapshot) normalized() Snapshot {
	s.Name = strings.TrimSpace(s.Name)
	if s.CID11 != nil {
		coding := s.CID11.normalized()
		s.CID11 = &coding
	}
	if s.MergedIntoID != nil {
		id := *s.MergedIntoID
		s.MergedIntoID = &id
	}
	return s
}

func (s Snapshot) Validate(problemID uuid.UUID) error {
	if strings.TrimSpace(s.Name) == "" {
		return ErrInvalidName
	}
	if s.CID11 != nil {
		if strings.TrimSpace(s.CID11.Code) == "" ||
			strings.TrimSpace(s.CID11.System) == "" ||
			strings.TrimSpace(s.CID11.Version) == "" {
			return ErrInvalidCID11
		}
	}
	if !s.Classification.IsValid() {
		return ErrInvalidClassification
	}
	if !s.ClinicalStatus.IsValid() {
		return ErrInvalidClinicalStatus
	}
	if !s.AdministrativeStatus.IsValid() {
		return ErrInvalidAdministrativeStatus
	}
	if s.Classification == ClassificationChronic && s.ClinicalStatus == ClinicalStatusResolved {
		return ErrChronicResolved
	}
	if s.AdministrativeStatus == AdministrativeStatusMerged {
		if s.MergedIntoID == nil || *s.MergedIntoID == uuid.Nil || *s.MergedIntoID == problemID {
			return ErrInvalidMergeTarget
		}
	} else if s.MergedIntoID != nil {
		return ErrInvalidMergeTarget
	}
	return nil
}
