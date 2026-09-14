package command

import (
	"time"

	"github.com/google/uuid"
)

type UserResult struct {
	ID        uuid.UUID
	Email     string
	Username  string
	Timezone  string
	CreatedAt time.Time
	UpdatedAt time.Time
}
