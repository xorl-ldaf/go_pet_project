package out

import (
	"context"
	"time"

	"go_pet_project/internal/reminder/domain"
)

type DueReminderHandler func(ctx context.Context, reminders []domain.Reminder) error

type DueReminderClaimer interface {
	ClaimDue(ctx context.Context, now time.Time, limit int, handle DueReminderHandler) (int, error)
}
