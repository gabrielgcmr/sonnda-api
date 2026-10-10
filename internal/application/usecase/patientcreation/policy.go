// internal/application/usecase/patientcreation/policy.go
package patientcreation

import (
	accountdomain "github.com/gabrielgcmr/sonnda/internal/features/account/domain"
	accessdomain "github.com/gabrielgcmr/sonnda/internal/features/patient/access/domain"
	"github.com/gabrielgcmr/sonnda/internal/kernel/apperr"
	"github.com/google/uuid"
)

// RequireInitialRelationship authorizes the relationship selected by the
// authenticated account when creating a patient.
func RequireInitialRelationship(
	account *accountdomain.Account,
	relationType accessdomain.RelationshipType,
) error {
	if account == nil || account.ID == uuid.Nil {
		return apperr.Unauthorized("autenticação necessária")
	}
	if account.DeletedAt != nil || !account.AccountType.IsValid() {
		return apperr.Forbidden("acesso negado")
	}
	if relationType == accessdomain.RelationshipTypeProfessional &&
		account.AccountType != accountdomain.AccountTypeProfessional {
		return apperr.Forbidden("acesso negado")
	}
	return nil
}
