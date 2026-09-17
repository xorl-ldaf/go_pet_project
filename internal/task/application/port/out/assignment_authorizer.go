package out

import (
	"context"

	"github.com/google/uuid"
)

type AssignmentAuthorizer interface {
	CanAssign(ctx context.Context, assignerID uuid.UUID, assigneeID uuid.UUID) (bool, error)
}
