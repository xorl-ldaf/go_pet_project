package in

import (
	"context"

	"go_pet_project/internal/notification/application/command"
	"go_pet_project/internal/notification/application/query"
	"go_pet_project/internal/notification/domain"
)

type NotificationService interface {
	HandleNotificationRequested(ctx context.Context, cmd command.HandleNotificationRequestedCommand) error
	CreateTelegramLinkToken(ctx context.Context, cmd command.CreateTelegramLinkCommand) (command.TelegramLinkTokenResult, error)
	HandleTelegramStart(ctx context.Context, cmd command.HandleTelegramStartCommand) error
	GetNotificationSettings(ctx context.Context, q query.GetNotificationSettingsQuery) (query.NotificationSettings, error)
	SetTelegramEnabled(ctx context.Context, cmd command.SetTelegramEnabledCommand) error
	DeleteTelegramLink(ctx context.Context, cmd command.DeleteTelegramLinkCommand) error
	ListNotifications(ctx context.Context, q query.ListNotificationsQuery) ([]domain.Notification, error)
	CountUnread(ctx context.Context, q query.CountUnreadQuery) (int, error)
	MarkRead(ctx context.Context, cmd command.MarkReadCommand) error
	MarkAllRead(ctx context.Context, cmd command.MarkAllReadCommand) error
}
