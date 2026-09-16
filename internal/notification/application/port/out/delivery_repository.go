package out

import (
	"context"
	"time"

	"go_pet_project/internal/notification/domain"

	"github.com/google/uuid"
)

type DeliveryRepository interface {
	Create(ctx context.Context, delivery domain.NotificationDelivery) (domain.NotificationDelivery, error)
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.NotificationDelivery, error)
	MarkSent(ctx context.Context, id uuid.UUID, attempts int, sentAt time.Time) error
	ScheduleRetry(ctx context.Context, id uuid.UUID, attempts int, nextAttemptAt time.Time, lastError string) error
	MarkFailed(ctx context.Context, id uuid.UUID, attempts int, lastError string) error
}
