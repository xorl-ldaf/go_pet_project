package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go_pet_project/internal/outbox/application/port/out"
	"go_pet_project/internal/outbox/domain"
	"go_pet_project/internal/platform/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ out.Repository = (*Repository)(nil)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, event domain.Event) (domain.Event, error) {
	model := toModel(event)
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Event{}, fmt.Errorf("create outbox event: %w", err)
	}

	created, err := toDomain(model)
	if err != nil {
		return domain.Event{}, fmt.Errorf("create outbox event: %w", err)
	}

	return created, nil
}

func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (domain.Event, error) {
	var model eventModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Event{}, mapFindError("find outbox event by id", err)
	}

	event, err := toDomain(model)
	if err != nil {
		return domain.Event{}, fmt.Errorf("find outbox event by id: %w", err)
	}

	return event, nil
}

func (r *Repository) ClaimUnpublished(ctx context.Context, limit int, handle out.EventHandler) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("claim unpublished outbox events: limit must be positive")
	}
	if handle == nil {
		return 0, fmt.Errorf("claim unpublished outbox events: handler is required")
	}

	claimed := 0
	err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var models []eventModel
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("published_at IS NULL").
			Order("created_at ASC, id ASC").
			Limit(limit).
			Find(&models).Error; err != nil {
			return fmt.Errorf("select unpublished outbox events: %w", err)
		}

		txCtx := database.ContextWithGORM(ctx, tx)
		for _, model := range models {
			event, err := toDomain(model)
			if err != nil {
				return fmt.Errorf("map unpublished outbox event: %w", err)
			}
			claimed++
			if err := handle(txCtx, event); err != nil {
				return fmt.Errorf("handle unpublished outbox event: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("claim unpublished outbox events: %w", err)
	}

	return claimed, nil
}

func (r *Repository) MarkPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&eventModel{}).
		Where("id = ? AND published_at IS NULL", id).
		Update("published_at", publishedAt)
	if result.Error != nil {
		return fmt.Errorf("mark outbox event published: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("mark outbox event published: %w", domain.ErrEventNotFound)
	}

	return nil
}

func (r *Repository) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&eventModel{}).
		Where("id = ? AND published_at IS NULL", id).
		Update("attempts", gorm.Expr("attempts + 1"))
	if result.Error != nil {
		return fmt.Errorf("increment outbox event attempts: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("increment outbox event attempts: %w", domain.ErrEventNotFound)
	}

	return nil
}

func toModel(event domain.Event) eventModel {
	return eventModel{
		ID:            event.ID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		Payload:       cloneJSON(event.Payload),
		CreatedAt:     event.CreatedAt,
		PublishedAt:   cloneTimePtr(event.PublishedAt),
		Attempts:      event.Attempts,
	}
}

func toDomain(model eventModel) (domain.Event, error) {
	return domain.RestoreEvent(
		model.ID,
		model.AggregateType,
		model.AggregateID,
		model.EventType,
		cloneJSON(model.Payload),
		model.CreatedAt,
		cloneTimePtr(model.PublishedAt),
		model.Attempts,
	)
}

func cloneJSON(value []byte) []byte {
	if value == nil {
		return nil
	}
	copied := make([]byte, len(value))
	copy(copied, value)

	return copied
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}

func mapFindError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, domain.ErrEventNotFound)
	}

	return fmt.Errorf("%s: %w", operation, err)
}
