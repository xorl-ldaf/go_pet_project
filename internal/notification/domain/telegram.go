package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type TelegramLink struct {
	UserID           uuid.UUID
	ChatID           int64
	TelegramUsername *string
	LinkedAt         time.Time
	Enabled          bool
}

func NewTelegramLink(userID uuid.UUID, chatID int64, username *string, linkedAt time.Time) (TelegramLink, error) {
	return RestoreTelegramLink(userID, chatID, username, linkedAt, true)
}

func RestoreTelegramLink(userID uuid.UUID, chatID int64, username *string, linkedAt time.Time, enabled bool) (TelegramLink, error) {
	if userID == uuid.Nil {
		return TelegramLink{}, ErrInvalidUserID
	}
	if chatID == 0 {
		return TelegramLink{}, ErrInvalidTelegramChatID
	}
	if linkedAt.IsZero() {
		return TelegramLink{}, ErrInvalidTelegramLinkedAt
	}
	if username != nil {
		trimmed := strings.TrimSpace(*username)
		if trimmed == "" {
			username = nil
		} else {
			username = &trimmed
		}
	}

	return TelegramLink{
		UserID:           userID,
		ChatID:           chatID,
		TelegramUsername: cloneStringPtr(username),
		LinkedAt:         linkedAt,
		Enabled:          enabled,
	}, nil
}

type TelegramLinkToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func NewTelegramLinkToken(id uuid.UUID, userID uuid.UUID, tokenHash string, expiresAt time.Time, createdAt time.Time) (TelegramLinkToken, error) {
	return RestoreTelegramLinkToken(id, userID, tokenHash, expiresAt, nil, createdAt)
}

func RestoreTelegramLinkToken(id uuid.UUID, userID uuid.UUID, tokenHash string, expiresAt time.Time, usedAt *time.Time, createdAt time.Time) (TelegramLinkToken, error) {
	if id == uuid.Nil {
		return TelegramLinkToken{}, ErrInvalidTelegramLinkTokenID
	}
	if userID == uuid.Nil {
		return TelegramLinkToken{}, ErrInvalidUserID
	}
	if strings.TrimSpace(tokenHash) == "" {
		return TelegramLinkToken{}, ErrInvalidTelegramLinkToken
	}
	if expiresAt.IsZero() {
		return TelegramLinkToken{}, ErrInvalidTelegramLinkTokenExpiry
	}
	if createdAt.IsZero() {
		return TelegramLinkToken{}, ErrInvalidCreatedAt
	}
	if usedAt != nil && usedAt.IsZero() {
		return TelegramLinkToken{}, ErrInvalidTelegramLinkTokenUsedAt
	}

	return TelegramLinkToken{
		ID:        id,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		UsedAt:    cloneTimePtr(usedAt),
		CreatedAt: createdAt,
	}, nil
}
