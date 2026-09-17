package postgres

import (
	"time"

	"github.com/google/uuid"
)

type taskModel struct {
	ID          uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	SeriesID    *uuid.UUID `gorm:"column:series_id;type:uuid"`
	CreatorID   uuid.UUID  `gorm:"column:creator_id;type:uuid;not null"`
	AssigneeID  uuid.UUID  `gorm:"column:assignee_id;type:uuid;not null"`
	Title       string     `gorm:"column:title;type:text;not null"`
	Description string     `gorm:"column:description;type:text;not null;default:''"`
	Status      string     `gorm:"column:status;type:text;not null"`
	DeadlineAt  *time.Time `gorm:"column:deadline_at;type:timestamptz"`
	CreatedAt   time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	CompletedAt *time.Time `gorm:"column:completed_at;type:timestamptz"`
	ArchivedAt  *time.Time `gorm:"column:archived_at;type:timestamptz"`
}

func (taskModel) TableName() string {
	return "tasks"
}
