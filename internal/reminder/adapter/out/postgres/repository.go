package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go_pet_project/internal/platform/database"
	reminderout "go_pet_project/internal/reminder/application/port/out"
	"go_pet_project/internal/reminder/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ reminderout.ReminderRepository = (*Repository)(nil)
var _ reminderout.DueReminderClaimer = (*Repository)(nil)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, reminder domain.Reminder) (domain.Reminder, error) {
	model := toModel(reminder)

	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Reminder{}, fmt.Errorf("create reminder: %w", err)
	}

	created, err := toDomain(model)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("create reminder: %w", err)
	}

	return created, nil
}

func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (domain.Reminder, error) {
	var model reminderModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Reminder{}, mapFindError("find reminder by id", err)
	}

	reminder, err := toDomain(model)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("find reminder by id: %w", err)
	}

	return reminder, nil
}

func (r *Repository) ListByTaskID(ctx context.Context, taskID uuid.UUID) ([]domain.Reminder, error) {
	var models []reminderModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("trigger_at ASC, created_at ASC, id ASC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list reminders by task id: %w", err)
	}

	reminders := make([]domain.Reminder, 0, len(models))
	for _, model := range models {
		reminder, err := toDomain(model)
		if err != nil {
			return nil, fmt.Errorf("list reminders by task id: %w", err)
		}
		reminders = append(reminders, reminder)
	}

	return reminders, nil
}

func (r *Repository) Update(ctx context.Context, reminder domain.Reminder) (domain.Reminder, error) {
	model := toModel(reminder)
	updates := map[string]any{
		"kind":           model.Kind,
		"offset_seconds": model.OffsetSeconds,
		"trigger_at":     model.TriggerAt,
		"state":          model.State,
		"sent_at":        model.SentAt,
	}

	db := database.GORMFromContext(ctx, r.db).WithContext(ctx)
	result := db.Model(&reminderModel{}).Where("id = ?", model.ID).Updates(updates)
	if result.Error != nil {
		return domain.Reminder{}, fmt.Errorf("update reminder: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.Reminder{}, fmt.Errorf("update reminder: %w", domain.ErrReminderNotFound)
	}

	return r.FindByID(ctx, model.ID)
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).Delete(&reminderModel{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete reminder: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("delete reminder: %w", domain.ErrReminderNotFound)
	}

	return nil
}

func (r *Repository) HasPendingBeforeDeadline(ctx context.Context, taskID uuid.UUID) (bool, error) {
	var exists bool
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Raw(`
			SELECT EXISTS (
				SELECT 1
				FROM reminders
				WHERE task_id = ?
					AND kind = ?
					AND state = ?
			)
		`, taskID, domain.KindBeforeDeadline, domain.StatePending).
		Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("check pending before-deadline reminders: %w", err)
	}

	return exists, nil
}

func (r *Repository) RecalculatePendingBeforeDeadline(ctx context.Context, taskID uuid.UUID, deadlineAt time.Time) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&reminderModel{}).
		Where("task_id = ? AND kind = ? AND state = ?", taskID, domain.KindBeforeDeadline, domain.StatePending).
		Update("trigger_at", gorm.Expr("?::timestamptz - (offset_seconds * INTERVAL '1 second')", deadlineAt))
	if result.Error != nil {
		return fmt.Errorf("recalculate pending before-deadline reminders: %w", result.Error)
	}

	return nil
}

func (r *Repository) CancelPendingByTaskID(ctx context.Context, taskID uuid.UUID) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&reminderModel{}).
		Where("task_id = ? AND state = ?", taskID, domain.StatePending).
		Update("state", domain.StateCancelled)
	if result.Error != nil {
		return fmt.Errorf("cancel pending reminders by task id: %w", result.Error)
	}

	return nil
}

func (r *Repository) ClaimDue(ctx context.Context, now time.Time, limit int, handle reminderout.DueReminderHandler) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("claim due reminders: limit must be positive")
	}
	if handle == nil {
		return 0, fmt.Errorf("claim due reminders: handler is required")
	}

	claimed := 0
	err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var models []reminderModel
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("state = ? AND trigger_at <= ?", domain.StatePending, now).
			Order("trigger_at ASC, id ASC").
			Limit(limit).
			Find(&models).Error; err != nil {
			return fmt.Errorf("select due reminders: %w", err)
		}

		reminders := make([]domain.Reminder, 0, len(models))
		for _, model := range models {
			reminder, err := toDomain(model)
			if err != nil {
				return fmt.Errorf("map due reminder: %w", err)
			}
			reminders = append(reminders, reminder)
		}

		claimed = len(reminders)
		if claimed == 0 {
			return nil
		}

		txCtx := database.ContextWithGORM(ctx, tx)
		if err := handle(txCtx, reminders); err != nil {
			return fmt.Errorf("handle due reminders: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("claim due reminders: %w", err)
	}

	return claimed, nil
}

func mapFindError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, domain.ErrReminderNotFound)
	}

	return fmt.Errorf("%s: %w", operation, err)
}
