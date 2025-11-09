package patient

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var (
	ErrPatientNotFound = errors.New("patient not found")
)

type Repository interface {
	// Operações CRUD básicas
	Create(ctx context.Context, patient *PatientProfile) error
	Update(ctx context.Context, patient *PatientProfile) error
	Delete(ctx context.Context, id uint) error
	List(ctx context.Context, limit, offset int) ([]PatientProfile, error)

	// Finders
	FindByUserID(ctx context.Context, userID uint) (*PatientProfile, error)
	FindByCPF(ctx context.Context, cpf string) (*PatientProfile, error)

	// Relacionamentos
	FindAuthorizations(ctx context.Context, patientID uint) ([]Authorization, error)
	CreateAuthorization(ctx context.Context, auth *Authorization) error
	UpdateAuthorization(ctx context.Context, auth *Authorization) error

	// Medical Records
	CreateMedicalRecord(ctx context.Context, record *MedicalRecord) error
	FindMedicalRecords(ctx context.Context, patientID uint) ([]MedicalRecord, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// Create: Cadastra um novo usuário
func (r *repository) Create(ctx context.Context, patient *PatientProfile) error {
	return r.db.WithContext(ctx).Create(patient).Error
}

// Update: Atualiza dados do paciente
func (r *repository) Update(ctx context.Context, patient *PatientProfile) error {
	return r.db.WithContext(ctx).Save(patient).Error
}

// Delete remove paciente (soft delete se configurado)
func (r *repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&PatientProfile{}, id).Error
}

// List retorna lista de pacientes com paginação
func (r *repository) List(ctx context.Context, limit, offset int) ([]PatientProfile, error) {
	var patients []PatientProfile
	err := r.db.WithContext(ctx).
		Limit(limit).
		Offset(offset).
		Find(&patients).Error
	return patients, err
}

// FindByUserID busca paciente por user_id
func (r *repository) FindByUserID(ctx context.Context, userID uint) (*PatientProfile, error) {
	var p PatientProfile
	if err := r.db.WithContext(ctx).
		Preload("Authorizations").
		Preload("MedicalRecords").
		First(&p, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPatientNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *repository) FindByCPF(ctx context.Context, cpf string) (*PatientProfile, error) {
	var p PatientProfile
	if err := r.db.WithContext(ctx).First(&p, "cpf = ?", cpf).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// FindAuthorizations retorna todas as autorizações do paciente
func (r *repository) FindAuthorizations(ctx context.Context, patientID uint) ([]Authorization, error) {
	var auths []Authorization
	err := r.db.WithContext(ctx).
		Where("patient_id = ?", patientID).
		Order("requested_at DESC").
		Find(&auths).Error
	return auths, err
}

// FindAuthorizationByUser busca autorização específica
func (r *repository) FindAuthorizationByUser(ctx context.Context, userID, patientID uint) (*Authorization, error) {
	var auth Authorization
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND patient_id = ?", userID, patientID).
		Order("requested_at DESC").
		First(&auth).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &auth, nil
}

// CreateAuthorization cria nova autorização
func (r *repository) CreateAuthorization(ctx context.Context, auth *Authorization) error {
	return r.db.WithContext(ctx).Create(auth).Error
}

// UpdateAuthorization atualiza autorização
func (r *repository) UpdateAuthorization(ctx context.Context, auth *Authorization) error {
	return r.db.WithContext(ctx).Save(auth).Error
}

// CreateMedicalRecord cria registro médico
func (r *repository) CreateMedicalRecord(ctx context.Context, record *MedicalRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// FindMedicalRecords retorna histórico médico do paciente
func (r *repository) FindMedicalRecords(ctx context.Context, patientID uint) ([]MedicalRecord, error) {
	var records []MedicalRecord
	err := r.db.WithContext(ctx).
		Where("patient_id = ?", patientID).
		Preload("PreventionData").
		Preload("ProblemData").
		Preload("ExamData").
		Preload("PhysicalExamData").
		Order("date DESC").
		Find(&records).Error
	return records, err
}
