// internal/features/capture/domain/session.go
package capturedomain

import (
	"time"

	"github.com/google/uuid"
)

const (
	CredentialHashBytes = 32
	MaxPairingLifetime  = 5 * time.Minute
	MaxUploadLifetime   = 12 * time.Hour
)

type Session struct {
	ID                   uuid.UUID
	AccountID            uuid.UUID
	PairingCodeHash      []byte
	PairingExpiresAt     time.Time
	UploadTokenHash      []byte
	UploadTokenExpiresAt *time.Time
	ClaimedAt            *time.Time
	DesktopLastSeenAt    time.Time
	MobileLastSeenAt     *time.Time
	RevokedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type SessionClaim struct {
	UploadTokenHash      []byte
	ClaimedAt            time.Time
	UploadTokenExpiresAt time.Time
}

func NewSession(accountID uuid.UUID, pairingCodeHash []byte, now time.Time) (Session, error) {
	createdAt := now.UTC()
	session := Session{
		ID:                uuid.New(),
		AccountID:         accountID,
		PairingCodeHash:   cloneBytes(pairingCodeHash),
		PairingExpiresAt:  createdAt.Add(MaxPairingLifetime),
		DesktopLastSeenAt: createdAt,
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}
	return session, session.Validate()
}

func (s Session) Validate() error {
	if s.ID == uuid.Nil {
		return ErrInvalidID
	}
	if s.AccountID == uuid.Nil {
		return ErrInvalidAccountID
	}
	if len(s.PairingCodeHash) != CredentialHashBytes ||
		(len(s.UploadTokenHash) != 0 && len(s.UploadTokenHash) != CredentialHashBytes) {
		return ErrInvalidHash
	}
	if s.CreatedAt.IsZero() || s.UpdatedAt.Before(s.CreatedAt) ||
		s.DesktopLastSeenAt.Before(s.CreatedAt) ||
		(s.RevokedAt != nil && s.RevokedAt.Before(s.CreatedAt)) {
		return ErrInvalidTimestamp
	}
	if !s.PairingExpiresAt.After(s.CreatedAt) ||
		s.PairingExpiresAt.After(s.CreatedAt.Add(MaxPairingLifetime)) {
		return ErrInvalidExpiration
	}
	claimed := s.ClaimedAt != nil || s.UploadTokenExpiresAt != nil || len(s.UploadTokenHash) != 0
	if !claimed {
		if s.MobileLastSeenAt != nil {
			return ErrInvalidClaim
		}
		return nil
	}
	if s.ClaimedAt == nil || s.UploadTokenExpiresAt == nil || len(s.UploadTokenHash) == 0 ||
		s.ClaimedAt.Before(s.CreatedAt) || !s.UploadTokenExpiresAt.After(*s.ClaimedAt) ||
		s.UploadTokenExpiresAt.After(s.ClaimedAt.Add(MaxUploadLifetime)) ||
		(s.MobileLastSeenAt != nil && s.MobileLastSeenAt.Before(*s.ClaimedAt)) {
		return ErrInvalidClaim
	}
	return nil
}

func (s Session) Claim(uploadTokenHash []byte, claimedAt time.Time) (Session, error) {
	when := claimedAt.UTC()
	claim, err := NewSessionClaim(uploadTokenHash, when)
	if err != nil || s.ClaimedAt != nil || s.RevokedAt != nil || !when.Before(s.PairingExpiresAt) {
		return Session{}, ErrInvalidClaim
	}
	next := s
	next.UploadTokenHash = cloneBytes(claim.UploadTokenHash)
	next.ClaimedAt = timePointer(claim.ClaimedAt)
	next.UploadTokenExpiresAt = timePointer(claim.UploadTokenExpiresAt)
	next.UpdatedAt = when
	return next, next.Validate()
}

func NewSessionClaim(uploadTokenHash []byte, claimedAt time.Time) (SessionClaim, error) {
	claim := SessionClaim{
		UploadTokenHash:      cloneBytes(uploadTokenHash),
		ClaimedAt:            claimedAt.UTC(),
		UploadTokenExpiresAt: claimedAt.UTC().Add(MaxUploadLifetime),
	}
	return claim, claim.Validate()
}

func (c SessionClaim) Validate() error {
	if len(c.UploadTokenHash) != CredentialHashBytes {
		return ErrInvalidHash
	}
	if c.ClaimedAt.IsZero() || !c.UploadTokenExpiresAt.After(c.ClaimedAt) ||
		c.UploadTokenExpiresAt.After(c.ClaimedAt.Add(MaxUploadLifetime)) {
		return ErrInvalidClaim
	}
	return nil
}

func cloneBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}

func timePointer(value time.Time) *time.Time {
	return &value
}
