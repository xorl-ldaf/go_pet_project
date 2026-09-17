package command

import "github.com/google/uuid"

type DeleteReminderCommand struct {
	ActorID    uuid.UUID
	TaskID     uuid.UUID
	ReminderID uuid.UUID
}
