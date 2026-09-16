package postgres

import (
	"context"
	"errors"
	"fmt"

	userout "go_pet_project/internal/user/application/port/out"
	"go_pet_project/internal/user/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
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
		return domain.User{}, mapCreateError(err)
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

func (r *Repository) FindByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.User, error) {
	if len(ids) == 0 {
		return []domain.User{}, nil
	}

	var models []userModel
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Order("username ASC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("find users by ids: %w", err)
	}

	users := make([]domain.User, 0, len(models))
	for _, model := range models {
		users = append(users, toDomain(model))
	}

	return users, nil
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

func mapCreateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_email_key":
			return fmt.Errorf("create user: %w", domain.ErrEmailAlreadyExists)
		case "users_username_key":
			return fmt.Errorf("create user: %w", domain.ErrUsernameAlreadyExists)
		}
	}

	return fmt.Errorf("create user: %w", err)
}
