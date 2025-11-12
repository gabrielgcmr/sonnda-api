package model

import "time"

type Role string

const (
	RolePatient Role = "PATIENT"
	RoleDoctor  Role = "DOCTOR"
	RoleAdmin   Role = "ADMIN"
)

type User struct {
	UID          uint      `gorm:"primaryKey"`
	Email        string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	Role         Role      `gorm:"type:varchar(20);not null"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
