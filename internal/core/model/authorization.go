package model

import "time"

// Sistema de autorizações com histórico
type Authorization struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	AuthorizedID uint       `gorm:"not null" json:"user_id"`
	PatientID    uint       `gorm:"not null" json:"patient_id"`
	Status       AuthStatus `gorm:"type:varchar(20);not null" json:"status"`
	RequestedAt  time.Time  `json:"requested_at"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`

	// Histórico de alterações
	History []AuthorizationHistory `gorm:"foreignKey:AuthorizationID" json:"history"`
}

type AuthStatus string

const (
	AuthPending  AuthStatus = "PENDING"
	AuthApproved AuthStatus = "APPROVED"
	AuthRevoked  AuthStatus = "REVOKED"
	AuthExpired  AuthStatus = "EXPIRED"
)

type AuthorizationHistory struct {
	ID              uint       `gorm:"primaryKey"`
	AuthorizationID uint       `gorm:"not null"`
	OldStatus       AuthStatus `gorm:"type:varchar(20)"`
	NewStatus       AuthStatus `gorm:"type:varchar(20);not null"`
	ChangedBy       uint       `gorm:"not null"` // UserID de quem mudou
	Reason          string     `json:"reason,omitempty"`
	ChangedAt       time.Time  `json:"changed_at"`
}
