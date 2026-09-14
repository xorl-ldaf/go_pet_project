package query

import (
	"time"

	"github.com/google/uuid"
)

type GetMeQuery struct {
	UserID uuid.UUID
}

type GetMeResult struct {
	ID        uuid.UUID
	Email     string
	Username  string
	Timezone  string
	CreatedAt time.Time
	UpdatedAt time.Time
}
