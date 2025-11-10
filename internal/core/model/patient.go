package model

import (
	"time"
)

type PatientProfile struct {
	UserID    uint       `gorm:"primaryKey" json:"user_id"`
	CPF       string     `gorm:"size:11;not null;uniqueIndex" json:"cpf"`
	CNS       *string    `gorm:"size:15" json:"cns,omitempty"`
	FullName  string     `gorm:"size:255;not null" json:"full_name"`
	BirthDate time.Time  `json:"birth_date"`
	Gender    Gender     `gorm:"type:varchar(20)" json:"gender"`
	Race      Race       `gorm:"type:varchar(50)" json:"race"`
	AvatarURL string     `json:"avatar_url"`
	Phone     *string    `gorm:"size:20" json:"phone,omitempty"`
	CreatedAt *time.Time `gorm:"autoCreateTime" json:"created_at,omitempty"`
	UpdatedAt *time.Time `gorm:"autoUpdateTime" json:"updated_at,omitempty"`

	// Relacionamentos
	MedicalRecords []MedicalRecord `gorm:"foreignKey:UserID" json:"medical_records"`
	Authorizations []Authorization `gorm:"foreignKey:PatientID" json:"authorizations"`
}

type Gender string

const (
	GenderMale    Gender = "MALE"
	GenderFemale  Gender = "FEMALE"
	GenderOther   Gender = "OTHER"
	GenderUnknown Gender = "UNKNOWN"
)

type Race string

const (
	RaceWhite      Race = "WHITE"
	RaceBlack      Race = "BLACK"
	RaceAsian      Race = "ASIAN"
	RaceMixed      Race = "MIXED"
	RaceIndigenous Race = "INDIGENOUS"
	RaceUnknown    Race = "UNKNOWN"
)

// Mantendo a mesma estrutura do seu Kotlin
type MedicalRecord struct {
	ID          uint              `gorm:"primaryKey" json:"id"`
	UserID      uint              `gorm:"not null" json:"user_id"`
	CreatedBy   uint              `gorm:"not null" json:"created_by"` // UserID de quem criou
	EntryType   MedicalRecordType `gorm:"type:varchar(50);not null" json:"entry_type"`
	Title       string            `gorm:"not null" json:"title"`
	Description string            `json:"description"`
	Date        time.Time         `gorm:"not null" json:"date"`
	CreatedAt   time.Time         `json:"created_at"`

	// Campos específicos por tipo
	PreventionData   *Prevention   `gorm:"foreignKey:MedicalRecordID" json:"prevention,omitempty"`
	ProblemData      *Problem      `gorm:"foreignKey:MedicalRecordID" json:"problem,omitempty"`
	ExamData         *Exam         `gorm:"foreignKey:MedicalRecordID" json:"exam,omitempty"`
	PhysicalExamData *PhysicalExam `gorm:"foreignKey:MedicalRecordID" json:"physical_exam,omitempty"`
}

type MedicalRecordType string

const (
	RecordTypePrevention   MedicalRecordType = "PREVENTION"
	RecordTypeProblem      MedicalRecordType = "PROBLEM"
	RecordTypeExam         MedicalRecordType = "EXAM"
	RecordTypePhysicalExam MedicalRecordType = "PHYSICAL_EXAM"
	RecordTypeNote         MedicalRecordType = "NOTE"
)

// Estruturas do seu Kotlin adaptadas
type Prevention struct {
	ID              uint   `gorm:"primaryKey" json:"id"`
	MedicalRecordID uint   `json:"medical_record_id"`
	Name            string `gorm:"not null" json:"name"`
	Abbreviation    string `json:"abbreviation,omitempty"`
	Value           string `json:"value,omitempty"`
	ReferenceValue  string `json:"reference_value,omitempty"`
	Unit            string `json:"unit,omitempty"`
	Classification  string `json:"classification,omitempty"`
	Description     string `json:"description,omitempty"`
	Other           string `json:"other,omitempty"`
}

type Problem struct {
	ID              uint   `gorm:"primaryKey" json:"id"`
	MedicalRecordID uint   `json:"medical_record_id"`
	Name            string `gorm:"not null" json:"name"`
	Abbreviation    string `json:"abbreviation,omitempty"`
	BodySystem      string `json:"body_system,omitempty"`
	Description     string `json:"description,omitempty"`
	Other           string `json:"other,omitempty"`
}

type Exam struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	MedicalRecordID uint      `json:"medical_record_id"`
	Name            string    `gorm:"not null" json:"name"`
	Abbreviation    string    `json:"abbreviation,omitempty"`
	Value           string    `json:"value,omitempty"`
	Unit            string    `json:"unit,omitempty"`
	Method          string    `json:"method,omitempty"`
	ReferenceRange  string    `json:"reference_range,omitempty"`
	Date            time.Time `json:"date,omitempty"`
}

type PhysicalExam struct {
	ID              uint   `gorm:"primaryKey" json:"id"`
	MedicalRecordID uint   `json:"medical_record_id"`
	SystolicBP      string `json:"systolic_bp"`
	DiastolicBP     string `json:"diastolic_bp"`
}
