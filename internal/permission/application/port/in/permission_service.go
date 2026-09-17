package in

import (
	"context"

	"github.com/google/uuid"
)

type PermissionService interface {
	CanAssign(ctx context.Context, assignerID uuid.UUID, assigneeID uuid.UUID) (bool, error)
	ListAssignableUserIDs(ctx context.Context, assignerID uuid.UUID) ([]uuid.UUID, error)
}
