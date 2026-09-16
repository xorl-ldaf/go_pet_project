package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	outtelegram "go_pet_project/internal/notification/adapter/out/telegram"
	"go_pet_project/internal/notification/application/command"
	notificationin "go_pet_project/internal/notification/application/port/in"
	"go_pet_project/internal/notification/domain"
)

const pollTimeout = 25 * time.Second

type Poller struct {
	client  *outtelegram.Client
	service notificationin.NotificationService
	logger  *slog.Logger
	offset  int
}

func NewPoller(client *outtelegram.Client, service notificationin.NotificationService, logger *slog.Logger) *Poller {
	if logger == nil {
		logger = slog.Default()
	}

	return &Poller{
		client:  client,
		service: service,
		logger:  logger.With("component", "telegram_link_poller"),
	}
}

func (p *Poller) Run(ctx context.Context) error {
	for {
		updates, err := p.client.GetUpdates(ctx, p.offset, pollTimeout)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			p.logger.Error("telegram polling failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				continue
			}
		}

		for _, update := range updates {
			if update.UpdateID >= p.offset {
				p.offset = update.UpdateID + 1
			}
			if err := p.handleUpdate(ctx, update); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				p.logger.Error("telegram update handling failed", "error", err)
			}
		}
	}
}

func (p *Poller) handleUpdate(ctx context.Context, update outtelegram.Update) error {
	if update.Message == nil || update.Message.Chat == nil {
		return nil
	}

	token, ok := extractStartToken(update.Message.Text)
	if !ok {
		return p.client.SendText(ctx, update.Message.Chat.ID, "Send /start <token> to link Telegram.")
	}

	var username *string
	if update.Message.From != nil && strings.TrimSpace(update.Message.From.Username) != "" {
		value := strings.TrimSpace(update.Message.From.Username)
		username = &value
	}

	err := p.service.HandleTelegramStart(ctx, command.HandleTelegramStartCommand{
		Token:            token,
		ChatID:           update.Message.Chat.ID,
		TelegramUsername: username,
	})
	if err != nil {
		if errors.Is(err, domain.ErrTelegramLinkTokenInvalidOrExpired) {
			return p.client.SendText(ctx, update.Message.Chat.ID, "Invalid or expired link token.")
		}

		return err
	}

	return p.client.SendText(ctx, update.Message.Chat.ID, "Telegram linked successfully.")
}

func extractStartToken(text string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) != 2 {
		return "", false
	}
	commandName := parts[0]
	if commandName != "/start" && !strings.HasPrefix(commandName, "/start@") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}

	return token, true
}
