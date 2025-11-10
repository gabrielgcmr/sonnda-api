package auth

import (
	"context"

	"sonnda-api/internal/core/model"

	"gorm.io/gorm"
)

type Repository interface {
	Create(ctx context.Context, u *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id uint) (*model.User, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *repository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).
		Where("email = ?", email).
		First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *repository) FindByID(ctx context.Context, id uint) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).
		First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}
