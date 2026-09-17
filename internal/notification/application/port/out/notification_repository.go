package out

import (
	"context"
	"time"

	"go_pet_project/internal/notification/domain"

	"github.com/google/uuid"
)

type NotificationRepository interface {
	Create(ctx context.Context, notification domain.Notification) (domain.Notification, error)
	FindByID(ctx context.Context, id uuid.UUID) (domain.Notification, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Notification, error)
	CountUnreadByUserID(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, id uuid.UUID, readAt time.Time) error
	MarkAllRead(ctx context.Context, userID uuid.UUID, readAt time.Time) error
}
