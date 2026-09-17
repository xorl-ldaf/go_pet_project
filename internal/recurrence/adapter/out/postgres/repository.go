package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	recurrenceout "go_pet_project/internal/recurrence/application/port/out"
	"go_pet_project/internal/recurrence/domain"

	"go_pet_project/internal/platform/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ recurrenceout.Repository = (*Repository)(nil)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, series domain.TaskSeries) (domain.TaskSeries, error) {
	model := seriesToModel(series)
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Create(&model).Error; err != nil {
		return domain.TaskSeries{}, fmt.Errorf("create task series: %w", err)
	}

	created, err := seriesToDomain(model)
	if err != nil {
		return domain.TaskSeries{}, fmt.Errorf("create task series: %w", err)
	}

	return created, nil
}

func (r *Repository) Update(ctx context.Context, series domain.TaskSeries) (domain.TaskSeries, error) {
	model := seriesToModel(series)
	updates := map[string]any{
		"creator_id":       model.CreatorID,
		"assignee_id":      model.AssigneeID,
		"title":            model.Title,
		"description":      model.Description,
		"frequency":        model.Frequency,
		"interval":         model.Interval,
		"next_deadline_at": model.NextDeadlineAt,
		"timezone":         model.Timezone,
		"ends_at":          model.EndsAt,
		"is_active":        model.IsActive,
		"updated_at":       model.UpdatedAt,
	}

	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).Model(&seriesModel{}).Where("id = ?", model.ID).Updates(updates)
	if result.Error != nil {
		return domain.TaskSeries{}, fmt.Errorf("update task series: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.TaskSeries{}, fmt.Errorf("update task series: %w", domain.ErrSeriesNotFound)
	}

	return r.FindByID(ctx, model.ID)
}

func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (domain.TaskSeries, error) {
	var model seriesModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.TaskSeries{}, mapFindError("find task series by id", err)
	}

	series, err := seriesToDomain(model)
	if err != nil {
		return domain.TaskSeries{}, fmt.Errorf("find task series by id: %w", err)
	}

	return series, nil
}

func (r *Repository) ClaimDue(ctx context.Context, now time.Time, limit int, handle recurrenceout.DueSeriesHandler) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("claim due task series: limit must be positive")
	}
	if handle == nil {
		return 0, fmt.Errorf("claim due task series: handler is required")
	}

	claimed := 0
	err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var models []seriesModel
		if err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("is_active = ? AND next_deadline_at <= ?", true, now).
			Order("next_deadline_at ASC, id ASC").
			Limit(limit).
			Find(&models).Error; err != nil {
			return fmt.Errorf("select due task series: %w", err)
		}

		series := make([]domain.TaskSeries, 0, len(models))
		for _, model := range models {
			item, err := seriesToDomain(model)
			if err != nil {
				return fmt.Errorf("map due task series: %w", err)
			}
			series = append(series, item)
		}

		claimed = len(series)
		if claimed == 0 {
			return nil
		}

		txCtx := database.ContextWithGORM(ctx, tx)
		if err := handle(txCtx, series); err != nil {
			return fmt.Errorf("handle due task series: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("claim due task series: %w", err)
	}

	return claimed, nil
}

func (r *Repository) ListReminderRules(ctx context.Context, seriesID uuid.UUID) ([]domain.ReminderRule, error) {
	var models []reminderRuleModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Where("series_id = ?", seriesID).
		Order("offset_seconds ASC, id ASC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list task series reminder rules: %w", err)
	}

	rules := make([]domain.ReminderRule, 0, len(models))
	for _, model := range models {
		rule, err := reminderRuleToDomain(model)
		if err != nil {
			return nil, fmt.Errorf("list task series reminder rules: %w", err)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func (r *Repository) CreateReminderRule(ctx context.Context, rule domain.ReminderRule) (domain.ReminderRule, error) {
	model := reminderRuleToModel(rule)
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Create(&model).Error; err != nil {
		return domain.ReminderRule{}, fmt.Errorf("create task series reminder rule: %w", err)
	}

	created, err := reminderRuleToDomain(model)
	if err != nil {
		return domain.ReminderRule{}, fmt.Errorf("create task series reminder rule: %w", err)
	}

	return created, nil
}

func mapFindError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, domain.ErrSeriesNotFound)
	}

	return fmt.Errorf("%s: %w", operation, err)
}
