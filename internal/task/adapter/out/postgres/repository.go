package postgres

import (
	"context"
	"errors"
	"fmt"

	taskout "go_pet_project/internal/task/application/port/out"
	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var _ taskout.TaskRepository = (*Repository)(nil)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	model, err := toModel(task)
	if err != nil {
		return domain.Task{}, err
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Task{}, fmt.Errorf("create task: %w", err)
	}

	created, err := toDomain(model)
	if err != nil {
		return domain.Task{}, fmt.Errorf("create task: %w", err)
	}

	return created, nil
}

func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (domain.Task, error) {
	var model taskModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Task{}, mapFindError("find task by id", err)
	}

	task, err := toDomain(model)
	if err != nil {
		return domain.Task{}, fmt.Errorf("find task by id: %w", err)
	}

	return task, nil
}

func (r *Repository) Update(ctx context.Context, task domain.Task) (domain.Task, error) {
	model, err := toModel(task)
	if err != nil {
		return domain.Task{}, err
	}

	updates := map[string]any{
		"series_id":    model.SeriesID,
		"assignee_id":  model.AssigneeID,
		"title":        model.Title,
		"description":  model.Description,
		"status":       model.Status,
		"deadline_at":  model.DeadlineAt,
		"updated_at":   model.UpdatedAt,
		"completed_at": model.CompletedAt,
		"archived_at":  model.ArchivedAt,
	}

	result := r.db.WithContext(ctx).Model(&taskModel{}).Where("id = ?", model.ID).Updates(updates)
	if result.Error != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.Task{}, fmt.Errorf("update task: %w", domain.ErrTaskNotFound)
	}

	return r.FindByID(ctx, model.ID)
}

func (r *Repository) List(ctx context.Context, filter taskout.TaskFilter) ([]domain.Task, error) {
	query, err := applyFilter(r.db.WithContext(ctx).Model(&taskModel{}), filter)
	if err != nil {
		return nil, err
	}

	var models []taskModel
	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	tasks := make([]domain.Task, 0, len(models))
	for _, model := range models {
		task, err := toDomain(model)
		if err != nil {
			return nil, fmt.Errorf("list tasks: %w", err)
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func mapFindError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, domain.ErrTaskNotFound)
	}

	return fmt.Errorf("%s: %w", operation, err)
}
