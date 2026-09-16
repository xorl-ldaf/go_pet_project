package command

import (
	"time"

	"github.com/google/uuid"
)

type HandleNotificationRequestedCommand struct {
	EventID         uuid.UUID
	RecipientUserID uuid.UUID
	TaskID          uuid.UUID
	ReminderID      uuid.UUID
	OccurredAt      time.Time
}
