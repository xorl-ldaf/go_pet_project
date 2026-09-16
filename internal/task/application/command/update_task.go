package command

import (
	"time"

	"github.com/google/uuid"
)

type DeadlineUpdate struct {
	Value *time.Time
}

type UpdateTaskCommand struct {
	ActorID     uuid.UUID
	TaskID      uuid.UUID
	AssigneeID  *uuid.UUID
	Title       *string
	Description *string
	DeadlineAt  *DeadlineUpdate
}
