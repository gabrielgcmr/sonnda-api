package patient

import (
	"context"
	"errors"
	"sonnda-api/internal/core/model"

	"gorm.io/gorm"
)

var (
	ErrPatientNotFound = errors.New("patient not found")
)

type Repository interface {
	// Operações CRUD básicas
	Create(ctx context.Context, patient *model.PatientProfile) error
	Update(ctx context.Context, patient *model.PatientProfile) error
	Delete(ctx context.Context, id uint) error
	List(ctx context.Context, limit, offset int) ([]model.PatientProfile, error)

	// Finders
	FindByUserID(ctx context.Context, userID uint) (*model.PatientProfile, error)
	FindByCPF(ctx context.Context, cpf string) (*model.PatientProfile, error)

	// Relacionamentos
	FindAuthorizations(ctx context.Context, patientID uint) ([]model.Authorization, error)
	CreateAuthorization(ctx context.Context, auth *model.Authorization) error
	UpdateAuthorization(ctx context.Context, auth *model.Authorization) error

	// Medical Records
	CreateMedicalRecord(ctx context.Context, record *model.MedicalRecord) error
	FindMedicalRecords(ctx context.Context, patientID uint) ([]model.MedicalRecord, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// Create: Cadastra um novo usuário
func (r *repository) Create(ctx context.Context, patient *model.PatientProfile) error {
	return r.db.WithContext(ctx).Create(patient).Error
}

// Update: Atualiza dados do paciente
func (r *repository) Update(ctx context.Context, patient *model.PatientProfile) error {
	return r.db.WithContext(ctx).Save(patient).Error
}

// Delete remove paciente (soft delete se configurado)
func (r *repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.PatientProfile{}, id).Error
}

// List retorna lista de pacientes com paginação
func (r *repository) List(ctx context.Context, limit, offset int) ([]model.PatientProfile, error) {
	var patients []model.PatientProfile
	err := r.db.WithContext(ctx).
		Limit(limit).
		Offset(offset).
		Find(&patients).Error
	return patients, err
}

// FindByUserID busca paciente por user_id
func (r *repository) FindByUserID(ctx context.Context, userID uint) (*model.PatientProfile, error) {
	var p model.PatientProfile
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

func (r *repository) FindByCPF(ctx context.Context, cpf string) (*model.PatientProfile, error) {
	var p model.PatientProfile
	if err := r.db.WithContext(ctx).First(&p, "cpf = ?", cpf).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// FindAuthorizations retorna todas as autorizações do paciente
func (r *repository) FindAuthorizations(ctx context.Context, patientID uint) ([]model.Authorization, error) {
	var auths []model.Authorization
	err := r.db.WithContext(ctx).
		Where("patient_id = ?", patientID).
		Order("requested_at DESC").
		Find(&auths).Error
	return auths, err
}

// FindAuthorizationByUser busca autorização específica
func (r *repository) FindAuthorizationByUser(ctx context.Context, userID, patientID uint) (*model.Authorization, error) {
	var auth model.Authorization
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
func (r *repository) CreateAuthorization(ctx context.Context, auth *model.Authorization) error {
	return r.db.WithContext(ctx).Create(auth).Error
}

// UpdateAuthorization atualiza autorização
func (r *repository) UpdateAuthorization(ctx context.Context, auth *model.Authorization) error {
	return r.db.WithContext(ctx).Save(auth).Error
}

// CreateMedicalRecord cria registro médico
func (r *repository) CreateMedicalRecord(ctx context.Context, record *model.MedicalRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// FindMedicalRecords retorna histórico médico do paciente
func (r *repository) FindMedicalRecords(ctx context.Context, patientID uint) ([]model.MedicalRecord, error) {
	var records []model.MedicalRecord
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
