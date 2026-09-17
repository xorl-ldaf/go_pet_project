package out

import (
	"context"

	"github.com/google/uuid"
)

type AssignableUserProvider interface {
	ListAssignableUserIDs(ctx context.Context, assignerID uuid.UUID) ([]uuid.UUID, error)
}
