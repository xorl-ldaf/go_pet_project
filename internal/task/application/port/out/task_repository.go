package out

import (
	"context"
	"errors"
	"time"

	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

var ErrInvalidTaskFilter = errors.New("invalid task filter")

type TaskRepository interface {
	Create(ctx context.Context, task domain.Task) (domain.Task, error)
	FindByID(ctx context.Context, id uuid.UUID) (domain.Task, error)
	Update(ctx context.Context, task domain.Task) (domain.Task, error)
	List(ctx context.Context, filter TaskFilter) ([]domain.Task, error)
}

type ArchivedMode int

const (
	ArchivedActiveOnly ArchivedMode = iota
	ArchivedOnly
	ArchivedAll
)

type TaskSort int

const (
	TaskSortCreatedAtDesc TaskSort = iota
	TaskSortCreatedAtAsc
	TaskSortDeadlineAtAsc
	TaskSortDeadlineAtDesc
)

type TaskFilter struct {
	VisibleTo    *uuid.UUID
	AssigneeID   *uuid.UUID
	CreatorID    *uuid.UUID
	Status       *domain.Status
	Archived     ArchivedMode
	DeadlineFrom *time.Time
	DeadlineTo   *time.Time
	Overdue      *bool
	Now          *time.Time
	Search       string
	Sort         TaskSort
}
