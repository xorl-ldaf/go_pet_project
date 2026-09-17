package out

import (
	"context"
	"time"

	"go_pet_project/internal/outbox/domain"

	"github.com/google/uuid"
)

type EventHandler func(ctx context.Context, event domain.Event) error

type Repository interface {
	Create(ctx context.Context, event domain.Event) (domain.Event, error)
	ClaimUnpublished(ctx context.Context, limit int, handle EventHandler) (int, error)
	MarkPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error
	IncrementAttempts(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (domain.Event, error)
}
