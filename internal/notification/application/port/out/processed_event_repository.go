package out

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ProcessedEventRepository interface {
	TryInsert(ctx context.Context, consumerName string, eventID uuid.UUID, processedAt time.Time) (bool, error)
	Exists(ctx context.Context, consumerName string, eventID uuid.UUID) (bool, error)
}
