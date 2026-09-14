package command

import (
	"time"

	"github.com/google/uuid"
)

type CreateTaskCommand struct {
	ActorID     uuid.UUID
	AssigneeID  uuid.UUID
	Title       string
	Description string
	DeadlineAt  *time.Time
}
