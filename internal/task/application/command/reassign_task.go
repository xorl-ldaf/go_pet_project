package command

import "github.com/google/uuid"

type ReassignTaskCommand struct {
	ActorID       uuid.UUID
	TaskID        uuid.UUID
	NewAssigneeID uuid.UUID
}
