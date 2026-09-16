package out

import (
	"context"

	"go_pet_project/internal/outbox/domain"
)

type Publisher interface {
	Publish(ctx context.Context, event domain.Event) error
}
