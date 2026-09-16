package domain

import (
	"time"

	"github.com/google/uuid"
)

type AssignmentPermission struct {
	AssignerID uuid.UUID
	AssigneeID uuid.UUID
	CreatedAt  time.Time
}
