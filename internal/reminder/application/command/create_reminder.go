package command

import (
	"time"

	"go_pet_project/internal/reminder/domain"

	"github.com/google/uuid"
)

type CreateReminderCommand struct {
	ActorID       uuid.UUID
	TaskID        uuid.UUID
	Kind          domain.Kind
	OffsetSeconds *int64
	TriggerAt     *time.Time
}
