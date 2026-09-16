package httpadapter

import (
	"time"

	"go_pet_project/internal/platform/httpx"
	"go_pet_project/internal/task/domain"
)

type TaskResponse struct {
	ID          string     `json:"id"`
	SeriesID    *string    `json:"series_id"`
	CreatorID   string     `json:"creator_id"`
	AssigneeID  string     `json:"assignee_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	DeadlineAt  *time.Time `json:"deadline_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
	ArchivedAt  *time.Time `json:"archived_at"`
	Overdue     bool       `json:"overdue"`
}

type TaskListResponse struct {
	Items []TaskResponse `json:"items"`
}

func newTaskResponse(task domain.Task, now time.Time) TaskResponse {
	var seriesID *string
	if task.SeriesID != nil {
		value := task.SeriesID.String()
		seriesID = &value
	}

	return TaskResponse{
		ID:          task.ID.String(),
		SeriesID:    seriesID,
		CreatorID:   task.CreatorID.String(),
		AssigneeID:  task.AssigneeID.String(),
		Title:       task.Title,
		Description: task.Description,
		Status:      string(task.Status),
		DeadlineAt:  task.DeadlineAt,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
		CompletedAt: task.CompletedAt,
		ArchivedAt:  task.ArchivedAt,
		Overdue:     task.IsOverdue(now),
	}
}

func newTaskListResponse(tasks []domain.Task, now time.Time) TaskListResponse {
	items := make([]TaskResponse, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, newTaskResponse(task, now))
	}

	return TaskListResponse{Items: items}
}

type ErrorResponse = httpx.ErrorResponse
type ErrorBody = httpx.ErrorBody
