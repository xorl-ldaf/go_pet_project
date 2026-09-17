package out

import (
	"context"

	"github.com/google/uuid"
)

type PermissionRepository interface {
	Exists(ctx context.Context, assignerID uuid.UUID, assigneeID uuid.UUID) (bool, error)
	ListAssigneeIDs(ctx context.Context, assignerID uuid.UUID) ([]uuid.UUID, error)
}
