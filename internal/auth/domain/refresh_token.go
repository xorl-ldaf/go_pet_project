package domain

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (t RefreshToken) IsExpired(now time.Time) bool {
	return !t.ExpiresAt.After(now)
}

func (t RefreshToken) IsRevoked() bool {
	return t.RevokedAt != nil
}

func (t RefreshToken) IsActive(now time.Time) bool {
	return !t.IsRevoked() && !t.IsExpired(now)
}

func (t *RefreshToken) Revoke(at time.Time) {
	if t.RevokedAt != nil {
		return
	}

	t.RevokedAt = &at
}
