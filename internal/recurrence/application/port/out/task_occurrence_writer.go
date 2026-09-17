package out

import (
	"context"

	"go_pet_project/internal/task/domain"
)

type TaskOccurrenceWriter interface {
	Create(ctx context.Context, task domain.Task) (domain.Task, error)
}
