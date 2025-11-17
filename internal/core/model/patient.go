package model

import (
	"time"
)

type Patient struct {
	ID        uint      `json:"id"`
	CPF       string    `json:"cpf"`
	CNS       *string   `json:"cns,omitempty"`
	FullName  string    `json:"full_name"`
	BirthDate time.Time `json:"birth_date"`
	Gender    Gender    `json:"gender"`
	Race      Race      `json:"race"`
	AvatarURL string    `json:"avatar_url"`
	Phone     *string   `json:"phone,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relacionamentos
	MedicalRecords []MedicalRecord `json:"medical_records"`
	Authorizations []Authorization `json:"authorizations"`
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
	ID        uint    `json:"id"`
	PatientID uint    `json:"patient_id"`
	Patient   Patient `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	CreatedBy   uint              `json:"created_by"` // UserID de quem criou
	EntryType   MedicalRecordType `json:"entry_type"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Date        time.Time         `json:"date"`
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
