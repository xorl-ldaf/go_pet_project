package postgres

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type eventModel struct {
	ID            uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	AggregateType string          `gorm:"column:aggregate_type;type:text;not null"`
	AggregateID   uuid.UUID       `gorm:"column:aggregate_id;type:uuid;not null"`
	EventType     string          `gorm:"column:event_type;type:text;not null"`
	Payload       json.RawMessage `gorm:"column:payload;type:jsonb;not null"`
	CreatedAt     time.Time       `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	PublishedAt   *time.Time      `gorm:"column:published_at;type:timestamptz"`
	Attempts      int             `gorm:"column:attempts;not null;default:0"`
}

func (eventModel) TableName() string {
	return "outbox_events"
}
