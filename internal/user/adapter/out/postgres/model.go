package postgres

import (
	"time"

	"github.com/google/uuid"
)

type userModel struct {
	ID           uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	Email        string    `gorm:"column:email;type:text;not null;unique"`
	Username     string    `gorm:"column:username;type:text;not null;unique"`
	PasswordHash string    `gorm:"column:password_hash;type:text;not null"`
	Timezone     string    `gorm:"column:timezone;type:text;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
}

func (userModel) TableName() string {
	return "users"
}
