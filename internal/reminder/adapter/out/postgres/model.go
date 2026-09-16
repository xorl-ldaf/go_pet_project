package postgres

import (
	"time"

	"github.com/google/uuid"
)

type reminderModel struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TaskID        uuid.UUID  `gorm:"column:task_id;type:uuid;not null"`
	Kind          string     `gorm:"column:kind;type:text;not null"`
	OffsetSeconds *int64     `gorm:"column:offset_seconds;type:bigint"`
	TriggerAt     time.Time  `gorm:"column:trigger_at;type:timestamptz;not null"`
	State         string     `gorm:"column:state;type:text;not null"`
	CreatedAt     time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	SentAt        *time.Time `gorm:"column:sent_at;type:timestamptz"`
}

func (reminderModel) TableName() string {
	return "reminders"
}
