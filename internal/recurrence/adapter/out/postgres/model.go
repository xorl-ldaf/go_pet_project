package postgres

import (
	"time"

	"github.com/google/uuid"
)

type seriesModel struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	CreatorID      uuid.UUID  `gorm:"column:creator_id;type:uuid;not null"`
	AssigneeID     uuid.UUID  `gorm:"column:assignee_id;type:uuid;not null"`
	Title          string     `gorm:"column:title;type:text;not null"`
	Description    string     `gorm:"column:description;type:text;not null;default:''"`
	Frequency      string     `gorm:"column:frequency;type:text;not null"`
	Interval       int        `gorm:"column:interval;not null"`
	NextDeadlineAt time.Time  `gorm:"column:next_deadline_at;type:timestamptz;not null"`
	Timezone       string     `gorm:"column:timezone;type:text;not null"`
	EndsAt         *time.Time `gorm:"column:ends_at;type:timestamptz"`
	IsActive       bool       `gorm:"column:is_active;not null"`
	CreatedAt      time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
}

func (seriesModel) TableName() string {
	return "task_series"
}

type reminderRuleModel struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	SeriesID      uuid.UUID `gorm:"column:series_id;type:uuid;not null"`
	OffsetSeconds int64     `gorm:"column:offset_seconds;type:bigint;not null"`
}

func (reminderRuleModel) TableName() string {
	return "task_series_reminder_rules"
}
