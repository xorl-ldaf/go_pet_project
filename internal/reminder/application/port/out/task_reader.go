package out

import (
	"context"

	taskdomain "go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

type TaskReader interface {
	FindByID(ctx context.Context, id uuid.UUID) (taskdomain.Task, error)
}
