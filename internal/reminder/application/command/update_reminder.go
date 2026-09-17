package command

import (
	"time"

	"github.com/google/uuid"
)

type UpdateReminderCommand struct {
	ActorID       uuid.UUID
	TaskID        uuid.UUID
	ReminderID    uuid.UUID
	OffsetSeconds *int64
	TriggerAt     *time.Time
}
