package query

import (
	"time"

	taskout "go_pet_project/internal/task/application/port/out"
	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

type ListScope int

const (
	ListScopeVisible ListScope = iota
	ListScopeAssignedToMe
	ListScopeCreatedByMe
)

type ListTasksQuery struct {
	ActorID      uuid.UUID
	Scope        ListScope
	Status       *domain.Status
	Archived     taskout.ArchivedMode
	DeadlineFrom *time.Time
	DeadlineTo   *time.Time
	Overdue      *bool
	Now          *time.Time
	Search       string
	Sort         taskout.TaskSort
}
