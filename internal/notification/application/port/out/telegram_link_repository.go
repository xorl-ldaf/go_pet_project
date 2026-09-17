package out

import (
	"context"
	"time"

	"go_pet_project/internal/notification/domain"

	"github.com/google/uuid"
)

type TelegramLinkRepository interface {
	FindByUserID(ctx context.Context, userID uuid.UUID) (domain.TelegramLink, error)
	Upsert(ctx context.Context, link domain.TelegramLink) (domain.TelegramLink, error)
	SetEnabled(ctx context.Context, userID uuid.UUID, enabled bool) error
	Delete(ctx context.Context, userID uuid.UUID) error
}

type TelegramLinkTokenRepository interface {
	Create(ctx context.Context, token domain.TelegramLinkToken) (domain.TelegramLinkToken, error)
	ConsumeByHash(ctx context.Context, tokenHash string, usedAt time.Time) (domain.TelegramLinkToken, error)
}
