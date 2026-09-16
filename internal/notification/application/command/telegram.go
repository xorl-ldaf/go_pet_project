package command

import (
	"time"

	"github.com/google/uuid"
)

type CreateTelegramLinkCommand struct {
	ActorID uuid.UUID
}

type TelegramLinkTokenResult struct {
	Token     string
	ExpiresAt time.Time
}

type HandleTelegramStartCommand struct {
	Token            string
	ChatID           int64
	TelegramUsername *string
}

type SetTelegramEnabledCommand struct {
	ActorID uuid.UUID
	Enabled bool
}

type DeleteTelegramLinkCommand struct {
	ActorID uuid.UUID
}
