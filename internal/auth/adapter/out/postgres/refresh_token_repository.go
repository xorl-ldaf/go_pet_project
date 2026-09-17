package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	authout "go_pet_project/internal/auth/application/port/out"
	"go_pet_project/internal/auth/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ authout.RefreshTokenRepository = (*RefreshTokenRepository)(nil)

type RefreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	model := toModel(token)

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.RefreshToken{}, fmt.Errorf("create refresh token: %w", err)
	}

	return toDomain(model), nil
}

func (r *RefreshTokenRepository) FindByHash(ctx context.Context, tokenHash string) (domain.RefreshToken, error) {
	var model refreshTokenModel
	if err := r.db.WithContext(ctx).First(&model, "token_hash = ?", tokenHash).Error; err != nil {
		return domain.RefreshToken{}, mapFindError("find refresh token by hash", err)
	}

	return toDomain(model), nil
}

func (r *RefreshTokenRepository) Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) (domain.RefreshToken, error) {
	var model refreshTokenModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, "token_hash = ?", tokenHash).Error; err != nil {
			return mapFindError("revoke refresh token", err)
		}
		if model.RevokedAt != nil {
			return nil
		}

		if err := tx.Model(&model).Update("revoked_at", revokedAt).Error; err != nil {
			return fmt.Errorf("update refresh token revoked_at: %w", err)
		}
		model.RevokedAt = &revokedAt

		return nil
	})
	if err != nil {
		return domain.RefreshToken{}, err
	}

	return toDomain(model), nil
}

func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldTokenHash string, newToken domain.RefreshToken, revokedAt time.Time) (domain.RefreshToken, error) {
	newModel := toModel(newToken)

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var oldModel refreshTokenModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&oldModel, "token_hash = ?", oldTokenHash).Error; err != nil {
			return mapFindError("rotate refresh token", err)
		}
		if oldModel.RevokedAt != nil {
			return fmt.Errorf("rotate refresh token: %w", domain.ErrRefreshTokenRevoked)
		}

		update := tx.Model(&refreshTokenModel{}).
			Where("token_hash = ? AND revoked_at IS NULL", oldTokenHash).
			Update("revoked_at", revokedAt)
		if update.Error != nil {
			return fmt.Errorf("revoke old refresh token: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("rotate refresh token: %w", domain.ErrRefreshTokenRevoked)
		}

		if err := tx.Create(&newModel).Error; err != nil {
			return fmt.Errorf("create rotated refresh token: %w", err)
		}

		return nil
	})
	if err != nil {
		return domain.RefreshToken{}, err
	}

	return toDomain(newModel), nil
}

func mapFindError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, domain.ErrRefreshTokenNotFound)
	}

	return fmt.Errorf("%s: %w", operation, err)
}
