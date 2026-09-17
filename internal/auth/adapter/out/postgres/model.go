package postgres

import (
	"time"

	"github.com/google/uuid"
)

type refreshTokenModel struct {
	ID        uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null"`
	TokenHash string     `gorm:"column:token_hash;type:text;not null;uniqueIndex:refresh_tokens_token_hash_key"`
	ExpiresAt time.Time  `gorm:"column:expires_at;type:timestamptz;not null"`
	RevokedAt *time.Time `gorm:"column:revoked_at;type:timestamptz"`
	CreatedAt time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
}

func (refreshTokenModel) TableName() string {
	return "refresh_tokens"
}
