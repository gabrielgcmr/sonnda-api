package exam

import (
	"time"

	"gorm.io/gorm"
)

// CodeSystem define os sistemas de codificação de exame
type CodeSystem string

const (
	CodeSystemLOINC CodeSystem = "LOINC"
	CodeSystemTUSS  CodeSystem = "TUSS"
	CodeSystemSUS   CodeSystem = "SUS"
)

const (
	StatusPending     ExamStatus = "pending"
	StatusProcessed   ExamStatus = "processed"
	StatusNeedsReview ExamStatus = "needs_review"
)

type ExamStatus string

type ExamMeta struct {
	RawText    *string     `gorm:"type:text"     json:"raw_text,omitempty"`
	FileLink   *string     `gorm:"size:255"      json:"file_link,omitempty"`
	Code       *string     `gorm:"size:50"       json:"code,omitempty"`
	CodeSystem *CodeSystem `gorm:"type:varchar(20)" json:"code_system,omitempty"`
}

// Exam representa um exame clínico associado a um paciente
// Inclui dados estruturados e referência a arquivos raw (texto e imagem/PDF)
type Exam struct {
	ID        uint `gorm:"primaryKey;autoIncrement" json:"id"`
	PatientID uint `gorm:"not null;index" json:"patient_id"`
	//LabInfo
	LabName      string `gorm:"size:30;not null" json:"lab_name"`
	CNES         string `gorm:"size:12;not null" json:"CNES"`
	RegistroCRBM string `gorm:"size:12;" json:"CRBM"` // Registro no CRBM
	//PatientINFO
	Paciente         string `gorm:"size:80;not null" json:"paciente"`
	Solicitante      string `gorm:"size:80;not null" json:"solicitante"`
	Codigo           string `gorm:"size:10;" json:"codigo"`
	DataDeNascimento string `gorm:"size:10;" json:"data_de_nascimento"`
	Idade            string `gorm:"size:3;" json:"idade"`
	Sexo             string `gorm:"size:1;" json:"sexo"`
	Convenio         string `gorm:"size:10;" json:"convenio"`
	DataDeColeta     string `gorm:"size:10;" json:"DataDeColeta"`

	Name         string  `gorm:"size:100;not null" json:"name"`
	Key          string  `gorm:"size:100;not null" json:"key"`
	Label        *string `gorm:"size:100" json:"label,omitempty"`       // Rótulo amigável
	Tags         *string `gorm:"size:100" json:"tags,omitempty"`        // Ex: LipidProfile
	Abbreviation *string `gorm:"size:50" json:"abbreviation,omitempty"` // Ex: Hemograma

	Method *string `gorm:"size:100" json:"method,omitempty"` // Método de realização

	Date         *time.Time  `gorm:"type:date" json:"date,omitempty"`     // Data do exame
	RawText      *string     `gorm:"type:text" json:"raw_text,omitempty"` // Texto bruto extraído (OCR)
	Status       ExamStatus  `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Observations *string     `gorm:"type:text" json:"observations,omitempty"`
	FileLink     *string     `gorm:"size:255" json:"file_link,omitempty"`           // URL/Path do arquivo de imagem ou PDF
	Code         *string     `gorm:"size:50" json:"code,omitempty"`                 // Código do exame
	CodeSystem   *CodeSystem `gorm:"type:varchar(20)" json:"code_system,omitempty"` // Sistema de codificação

	Results []AnalitoResult `gorm:"foreignKey:ExamID;constraint:OnDelete:CASCADE" json:"results"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // Soft delete
	Metadata  ExamMeta       `gorm:"constraint:OnDelete:CASCADE" json:"metadata"`
}

// AnalitoResult representa cada “linha” do exame, ou seja, um analito e seu valor
type AnalitoResult struct {
	ID           uint    `gorm:"primaryKey" json:"id"`
	ExamID       uint    `gorm:"not null;index" json:"exam_id"`
	Name         string  `gorm:"size:100;not null" json:"name"`
	Abbreviation *string `gorm:"size:50" json:"abbreviation,omitempty"` // Ex: LDL
	// Para suportar valor numérico ou textual
	ValueString  *string  `gorm:"size:50" json:"value_string,omitempty"`
	ValueNumeric *float64 `json:"value_numeric,omitempty"`

	Unit     *string  `gorm:"size:20" json:"unit,omitempty"`
	MinValue *float64 `json:"min_value,omitempty"`
	MaxValue *float64 `json:"max_value,omitempty"`
	UnitRef  *string  `gorm:"size:20" json:"unit_ref,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type Laboratorio struct {
	gorm.Model
	CNES     string `gorm:"unique;not null"` // Código CNES
	Nome     string `gorm:"not null"`
	Endereco string
	Telefone string
}

type ValorReferencia struct {
	gorm.Model
	TipoExame string  `gorm:"not null"` // Ex: "Hemograma", "Glicose"
	IdadeMin  int     `gorm:"not null"` // Idade mínima
	IdadeMax  int     `gorm:"not null"` // Idade máxima
	Sexo      string  `gorm:"size:1"`   // "F", "M", ou "" (ambos)
	Parametro string  `gorm:"not null"` // Ex: "Hemoglobina", "LDL"
	ValorMin  float64 // Valor mínimo
	ValorMax  float64 // Valor máximo
}

type OCRResult struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

type ExameRaw struct {
	OCR    OCRResult      `json:"ocr_raw"`
	Parsed map[string]any `json:"parsed_fields"` // Campos extraídos
}
