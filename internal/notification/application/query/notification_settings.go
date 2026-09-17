package query

import "github.com/google/uuid"

type GetNotificationSettingsQuery struct {
	ActorID uuid.UUID
}

type NotificationSettings struct {
	Internal bool
	Telegram TelegramSettings
}

type TelegramSettings struct {
	Linked   bool
	Enabled  bool
	Username *string
}
