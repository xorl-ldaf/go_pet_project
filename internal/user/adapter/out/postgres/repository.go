package postgres

import (
	"context"
	"errors"
	"fmt"

	userout "go_pet_project/internal/user/application/port/out"
	"go_pet_project/internal/user/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var _ userout.UserRepository = (*Repository)(nil)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	model := toModel(user)

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}

	return toDomain(model), nil
}

func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	var model userModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.User{}, mapFindError("find user by id", err)
	}

	return toDomain(model), nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	var model userModel
	if err := r.db.WithContext(ctx).First(&model, "email = ?", email).Error; err != nil {
		return domain.User{}, mapFindError("find user by email", err)
	}

	return toDomain(model), nil
}

func (r *Repository) FindByUsername(ctx context.Context, username string) (domain.User, error) {
	var model userModel
	if err := r.db.WithContext(ctx).First(&model, "username = ?", username).Error; err != nil {
		return domain.User{}, mapFindError("find user by username", err)
	}

	return toDomain(model), nil
}

func mapFindError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, domain.ErrUserNotFound)
	}

	return fmt.Errorf("%s: %w", operation, err)
}
