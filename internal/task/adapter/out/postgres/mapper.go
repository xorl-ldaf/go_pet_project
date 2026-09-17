package postgres

import (
	"fmt"

	"go_pet_project/internal/task/domain"
)

func toModel(task domain.Task) (taskModel, error) {
	if !task.Status.IsValid() {
		return taskModel{}, fmt.Errorf("map task to model: %w: %s", domain.ErrInvalidStatus, task.Status)
	}

	return taskModel{
		ID:          task.ID,
		SeriesID:    cloneUUIDPtr(task.SeriesID),
		CreatorID:   task.CreatorID,
		AssigneeID:  task.AssigneeID,
		Title:       task.Title,
		Description: task.Description,
		Status:      string(task.Status),
		DeadlineAt:  cloneTimePtr(task.DeadlineAt),
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
		CompletedAt: cloneTimePtr(task.CompletedAt),
		ArchivedAt:  cloneTimePtr(task.ArchivedAt),
	}, nil
}

func toDomain(model taskModel) (domain.Task, error) {
	status, err := domain.ParseStatus(model.Status)
	if err != nil {
		return domain.Task{}, fmt.Errorf("map task model to domain: %w", err)
	}

	task, err := domain.RestoreTask(
		model.ID,
		cloneUUIDPtr(model.SeriesID),
		model.CreatorID,
		model.AssigneeID,
		model.Title,
		model.Description,
		status,
		cloneTimePtr(model.DeadlineAt),
		model.CreatedAt,
		model.UpdatedAt,
		cloneTimePtr(model.CompletedAt),
		cloneTimePtr(model.ArchivedAt),
	)
	if err != nil {
		return domain.Task{}, fmt.Errorf("map task model to domain: %w", err)
	}

	return task, nil
}
