package out

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ReminderManager interface {
	RecalculatePendingBeforeDeadline(ctx context.Context, taskID uuid.UUID, deadlineAt *time.Time) error
	CancelPending(ctx context.Context, taskID uuid.UUID) error
}
