package model

import "time"

type Role string

const (
	RolePatient Role = "PATIENT"
	RoleDoctor  Role = "DOCTOR"
	RoleAdmin   Role = "ADMIN"
)

type User struct {
	ID           uint      `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
