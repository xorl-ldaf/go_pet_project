package postgres

import (
	"time"

	"github.com/google/uuid"
)

type notificationModel struct {
	ID         uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID     uuid.UUID  `gorm:"column:user_id;type:uuid;not null"`
	TaskID     *uuid.UUID `gorm:"column:task_id;type:uuid"`
	ReminderID *uuid.UUID `gorm:"column:reminder_id;type:uuid"`
	Type       string     `gorm:"column:type;type:text;not null"`
	Title      string     `gorm:"column:title;type:text;not null"`
	Body       string     `gorm:"column:body;type:text;not null"`
	CreatedAt  time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	ReadAt     *time.Time `gorm:"column:read_at;type:timestamptz"`
}

func (notificationModel) TableName() string {
	return "notifications"
}

type processedEventModel struct {
	ConsumerName string    `gorm:"column:consumer_name;type:text;primaryKey"`
	EventID      uuid.UUID `gorm:"column:event_id;type:uuid;primaryKey"`
	ProcessedAt  time.Time `gorm:"column:processed_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
}

func (processedEventModel) TableName() string {
	return "processed_events"
}

type telegramLinkModel struct {
	UserID           uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey"`
	ChatID           int64     `gorm:"column:chat_id;type:bigint;not null"`
	TelegramUsername *string   `gorm:"column:telegram_username;type:text"`
	LinkedAt         time.Time `gorm:"column:linked_at;type:timestamptz;not null"`
	Enabled          bool      `gorm:"column:enabled;not null;default:true"`
}

func (telegramLinkModel) TableName() string {
	return "telegram_links"
}

type telegramLinkTokenModel struct {
	ID        uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null"`
	TokenHash string     `gorm:"column:token_hash;type:text;not null;unique"`
	ExpiresAt time.Time  `gorm:"column:expires_at;type:timestamptz;not null"`
	UsedAt    *time.Time `gorm:"column:used_at;type:timestamptz"`
	CreatedAt time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
}

func (telegramLinkTokenModel) TableName() string {
	return "telegram_link_tokens"
}

type notificationDeliveryModel struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	NotificationID uuid.UUID  `gorm:"column:notification_id;type:uuid;not null"`
	Channel        string     `gorm:"column:channel;type:text;not null"`
	Status         string     `gorm:"column:status;type:text;not null"`
	Attempts       int        `gorm:"column:attempts;not null;default:0"`
	NextAttemptAt  *time.Time `gorm:"column:next_attempt_at;type:timestamptz"`
	SentAt         *time.Time `gorm:"column:sent_at;type:timestamptz"`
	LastError      *string    `gorm:"column:last_error;type:text"`
}

func (notificationDeliveryModel) TableName() string {
	return "notification_deliveries"
}
