// internal/features/patient/problem/domain/transition.go
package problemdomain

import "github.com/google/uuid"

func validateTransition(problemID uuid.UUID, before, after Snapshot, action Action, reason string, sourceIDs []uuid.UUID) error {
	if before.AdministrativeStatus != AdministrativeStatusValid {
		return ErrInvalidTransition
	}
	if action != ActionMergedDestination && len(sourceIDs) != 0 {
		return ErrInvalidMergeSources
	}
	if action != ActionMergedSource && action != ActionRectified && after.AdministrativeStatus != AdministrativeStatusValid {
		return ErrInvalidTransition
	}

	switch action {
	case ActionEdited:
		if !sameLifecycle(before, after) || before.Classification != after.Classification || sameDetails(before, after) {
			return ErrInvalidTransition
		}
	case ActionClassified:
		if !sameLifecycle(before, after) || !sameDetails(before, after) || before.Classification == after.Classification {
			return ErrInvalidTransition
		}
	case ActionResolved:
		if before.Classification != ClassificationAcute || before.ClinicalStatus != ClinicalStatusActive || after.ClinicalStatus != ClinicalStatusResolved ||
			before.Classification != after.Classification || !sameAdministrative(before, after) || !sameDetails(before, after) {
			return ErrInvalidTransition
		}
	case ActionReopened:
		if before.ClinicalStatus != ClinicalStatusResolved || after.ClinicalStatus != ClinicalStatusActive ||
			before.Classification != after.Classification || !sameAdministrative(before, after) || !sameDetails(before, after) {
			return ErrInvalidTransition
		}
	case ActionRectified:
		if reason == "" {
			return ErrReasonRequired
		}
		if after.AdministrativeStatus != AdministrativeStatusEnteredInError || !sameClinicalFields(before, after) || after.MergedIntoID != nil {
			return ErrInvalidTransition
		}
	case ActionMergedSource:
		if after.AdministrativeStatus != AdministrativeStatusMerged || !sameClinicalFields(before, after) {
			return ErrInvalidTransition
		}
	case ActionMergedDestination:
		if err := validateMergeSources(problemID, sourceIDs); err != nil {
			return err
		}
		// The professional chooses every final field. B5 checks each source's
		// patient and saves all problem versions in one transaction.
	default:
		return ErrInvalidTransition
	}
	return nil
}

func validateMergeSources(destinationID uuid.UUID, sourceIDs []uuid.UUID) error {
	if len(sourceIDs) == 0 {
		return ErrInvalidMergeSources
	}
	seen := make(map[uuid.UUID]struct{}, len(sourceIDs))
	for _, id := range sourceIDs {
		if id == uuid.Nil || id == destinationID {
			return ErrInvalidMergeSources
		}
		if _, exists := seen[id]; exists {
			return ErrInvalidMergeSources
		}
		seen[id] = struct{}{}
	}
	return nil
}

func sameDetails(a, b Snapshot) bool {
	if a.Name != b.Name {
		return false
	}
	if a.CID11 == nil || b.CID11 == nil {
		return a.CID11 == nil && b.CID11 == nil
	}
	return *a.CID11 == *b.CID11
}

func sameAdministrative(a, b Snapshot) bool {
	return a.AdministrativeStatus == b.AdministrativeStatus && a.MergedIntoID == nil && b.MergedIntoID == nil
}

func sameLifecycle(a, b Snapshot) bool {
	return a.ClinicalStatus == b.ClinicalStatus && sameAdministrative(a, b)
}

func sameClinicalFields(a, b Snapshot) bool {
	return sameDetails(a, b) && a.Classification == b.Classification && a.ClinicalStatus == b.ClinicalStatus
}
