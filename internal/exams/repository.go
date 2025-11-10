package exam

import (
	"context"

	"sonnda-api/internal/core/model"

	"gorm.io/gorm"
)

type Repository interface {
	SaveMedicalRecord(ctx context.Context, record *model.MedicalRecord) error
	SaveExam(ctx context.Context, exam *model.Exam) error
	ListByPatient(ctx context.Context, patientID uint) ([]model.Exam, error)
	GetByID(ctx context.Context, id uint) (*model.Exam, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db}
}

func (r *repository) SaveMedicalRecord(ctx context.Context, record *model.MedicalRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *repository) SaveExam(ctx context.Context, exam *model.Exam) error {
	return r.db.WithContext(ctx).Create(exam).Error
}

func (r *repository) ListByPatient(ctx context.Context, patientID uint) ([]model.Exam, error) {
	var exams []model.Exam
	err := r.db.WithContext(ctx).
		Joins("JOIN medical_records ON medical_records.id = exams.medical_record_id").
		Where("medical_records.patient_id = ?", patientID).
		Find(&exams).Error
	return exams, err
}

func (r *repository) GetByID(ctx context.Context, id uint) (*model.Exam, error) {
	var exam model.Exam
	err := r.db.WithContext(ctx).First(&exam, id).Error
	if err != nil {
		return nil, err
	}
	return &exam, nil
}
