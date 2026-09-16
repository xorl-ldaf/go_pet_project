package out

import (
	"context"
	"time"

	"go_pet_project/internal/reminder/domain"

	"github.com/google/uuid"
)

type ReminderRepository interface {
	Create(ctx context.Context, reminder domain.Reminder) (domain.Reminder, error)
	FindByID(ctx context.Context, id uuid.UUID) (domain.Reminder, error)
	ListByTaskID(ctx context.Context, taskID uuid.UUID) ([]domain.Reminder, error)
	Update(ctx context.Context, reminder domain.Reminder) (domain.Reminder, error)
	Delete(ctx context.Context, id uuid.UUID) error
	HasPendingBeforeDeadline(ctx context.Context, taskID uuid.UUID) (bool, error)
	RecalculatePendingBeforeDeadline(ctx context.Context, taskID uuid.UUID, deadlineAt time.Time) error
	CancelPendingByTaskID(ctx context.Context, taskID uuid.UUID) error
}
