package out

import (
	"context"

	"go_pet_project/internal/reminder/domain"
)

type ReminderOccurrenceWriter interface {
	Create(ctx context.Context, reminder domain.Reminder) (domain.Reminder, error)
}
