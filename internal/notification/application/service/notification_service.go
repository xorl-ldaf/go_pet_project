package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go_pet_project/internal/notification/application"
	"go_pet_project/internal/notification/application/command"
	notificationin "go_pet_project/internal/notification/application/port/in"
	notificationout "go_pet_project/internal/notification/application/port/out"
	"go_pet_project/internal/notification/application/query"
	"go_pet_project/internal/notification/domain"
	"go_pet_project/internal/platform/metrics"

	"github.com/google/uuid"
)

const ConsumerName = "todo-notifier-v1"

const (
	telegramLinkTokenBytes = 32
	telegramLinkTokenTTL   = 10 * time.Minute
)

var _ notificationin.NotificationService = (*NotificationService)(nil)

type NotificationService struct {
	notifications   notificationout.NotificationRepository
	processedEvents notificationout.ProcessedEventRepository
	deliveries      notificationout.DeliveryRepository
	telegramLinks   notificationout.TelegramLinkRepository
	telegramTokens  notificationout.TelegramLinkTokenRepository
	transactions    notificationout.TransactionRunner
	consumerName    string
	now             func() time.Time
}

type Option func(*NotificationService)

func WithTelegramRepositories(
	deliveries notificationout.DeliveryRepository,
	telegramLinks notificationout.TelegramLinkRepository,
	telegramTokens notificationout.TelegramLinkTokenRepository,
) Option {
	return func(s *NotificationService) {
		s.deliveries = deliveries
		s.telegramLinks = telegramLinks
		s.telegramTokens = telegramTokens
	}
}

func NewNotificationService(
	notifications notificationout.NotificationRepository,
	processedEvents notificationout.ProcessedEventRepository,
	transactions notificationout.TransactionRunner,
	consumerName string,
	options ...Option,
) (*NotificationService, error) {
	if notifications == nil {
		return nil, errors.New("notification repository is required")
	}
	if processedEvents == nil {
		return nil, errors.New("processed event repository is required")
	}
	if transactions == nil {
		return nil, errors.New("transaction runner is required")
	}
	if consumerName == "" {
		return nil, errors.New("consumer name is required")
	}

	service := &NotificationService{
		notifications:   notifications,
		processedEvents: processedEvents,
		transactions:    transactions,
		consumerName:    consumerName,
		now:             time.Now,
	}
	for _, option := range options {
		option(service)
	}

	return service, nil
}

func (s *NotificationService) HandleNotificationRequested(ctx context.Context, cmd command.HandleNotificationRequestedCommand) error {
	if cmd.EventID == uuid.Nil ||
		cmd.RecipientUserID == uuid.Nil ||
		cmd.TaskID == uuid.Nil ||
		cmd.ReminderID == uuid.Nil ||
		cmd.OccurredAt.IsZero() {
		return application.ErrInvalidNotificationEvent
	}

	return s.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		now := s.now().UTC()
		inserted, err := s.processedEvents.TryInsert(txCtx, s.consumerName, cmd.EventID, now)
		if err != nil {
			return fmt.Errorf("register processed event: %w", err)
		}
		if !inserted {
			return nil
		}

		notification, err := domain.NewNotification(
			uuid.New(),
			cmd.RecipientUserID,
			&cmd.TaskID,
			&cmd.ReminderID,
			domain.TypeTaskReminder,
			"Task reminder",
			"A task reminder is due.",
			now,
		)
		if err != nil {
			return err
		}
		created, err := s.notifications.Create(txCtx, notification)
		if err != nil {
			return fmt.Errorf("create notification: %w", err)
		}
		metrics.ObserveNotificationCreated()
		if err := s.createTelegramDeliveryIfLinked(txCtx, created, now); err != nil {
			return err
		}

		return nil
	})
}

func (s *NotificationService) CreateTelegramLinkToken(ctx context.Context, cmd command.CreateTelegramLinkCommand) (command.TelegramLinkTokenResult, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return command.TelegramLinkTokenResult{}, err
	}
	if s.telegramTokens == nil {
		return command.TelegramLinkTokenResult{}, errors.New("telegram link token repository is required")
	}

	rawToken, tokenHash, err := generateTelegramLinkToken()
	if err != nil {
		return command.TelegramLinkTokenResult{}, err
	}

	now := s.now().UTC()
	expiresAt := now.Add(telegramLinkTokenTTL)
	token, err := domain.NewTelegramLinkToken(uuid.New(), cmd.ActorID, tokenHash, expiresAt, now)
	if err != nil {
		return command.TelegramLinkTokenResult{}, err
	}
	if _, err := s.telegramTokens.Create(ctx, token); err != nil {
		return command.TelegramLinkTokenResult{}, fmt.Errorf("create telegram link token: %w", err)
	}

	return command.TelegramLinkTokenResult{
		Token:     rawToken,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *NotificationService) HandleTelegramStart(ctx context.Context, cmd command.HandleTelegramStartCommand) error {
	rawToken := strings.TrimSpace(cmd.Token)
	if rawToken == "" || cmd.ChatID == 0 {
		return application.ErrInvalidTelegramToken
	}
	if s.telegramTokens == nil {
		return errors.New("telegram link token repository is required")
	}
	if s.telegramLinks == nil {
		return errors.New("telegram link repository is required")
	}

	return s.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		now := s.now().UTC()
		token, err := s.telegramTokens.ConsumeByHash(txCtx, hashTelegramLinkToken(rawToken), now)
		if err != nil {
			if errors.Is(err, domain.ErrTelegramLinkTokenNotFound) {
				return domain.ErrTelegramLinkTokenInvalidOrExpired
			}
			return fmt.Errorf("consume telegram link token: %w", err)
		}

		link, err := domain.NewTelegramLink(token.UserID, cmd.ChatID, cmd.TelegramUsername, now)
		if err != nil {
			return err
		}
		if _, err := s.telegramLinks.Upsert(txCtx, link); err != nil {
			return fmt.Errorf("upsert telegram link: %w", err)
		}

		return nil
	})
}

func (s *NotificationService) GetNotificationSettings(ctx context.Context, q query.GetNotificationSettingsQuery) (query.NotificationSettings, error) {
	if err := requireActor(q.ActorID); err != nil {
		return query.NotificationSettings{}, err
	}

	settings := query.NotificationSettings{Internal: true}
	if s.telegramLinks == nil {
		return settings, nil
	}

	link, err := s.telegramLinks.FindByUserID(ctx, q.ActorID)
	if err != nil {
		if errors.Is(err, domain.ErrTelegramLinkNotFound) {
			return settings, nil
		}
		return query.NotificationSettings{}, fmt.Errorf("find telegram link: %w", err)
	}

	settings.Telegram = query.TelegramSettings{
		Linked:   true,
		Enabled:  link.Enabled,
		Username: cloneStringPtr(link.TelegramUsername),
	}

	return settings, nil
}

func (s *NotificationService) SetTelegramEnabled(ctx context.Context, cmd command.SetTelegramEnabledCommand) error {
	if err := requireActor(cmd.ActorID); err != nil {
		return err
	}
	if s.telegramLinks == nil {
		return errors.New("telegram link repository is required")
	}
	if err := s.telegramLinks.SetEnabled(ctx, cmd.ActorID, cmd.Enabled); err != nil {
		return fmt.Errorf("set telegram enabled: %w", err)
	}

	return nil
}

func (s *NotificationService) DeleteTelegramLink(ctx context.Context, cmd command.DeleteTelegramLinkCommand) error {
	if err := requireActor(cmd.ActorID); err != nil {
		return err
	}
	if s.telegramLinks == nil {
		return errors.New("telegram link repository is required")
	}
	if err := s.telegramLinks.Delete(ctx, cmd.ActorID); err != nil {
		return fmt.Errorf("delete telegram link: %w", err)
	}

	return nil
}

func (s *NotificationService) ListNotifications(ctx context.Context, q query.ListNotificationsQuery) ([]domain.Notification, error) {
	if err := requireActor(q.ActorID); err != nil {
		return nil, err
	}

	notifications, err := s.notifications.ListByUserID(ctx, q.ActorID)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}

	return notifications, nil
}

func (s *NotificationService) CountUnread(ctx context.Context, q query.CountUnreadQuery) (int, error) {
	if err := requireActor(q.ActorID); err != nil {
		return 0, err
	}

	count, err := s.notifications.CountUnreadByUserID(ctx, q.ActorID)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}

	return count, nil
}

func (s *NotificationService) MarkRead(ctx context.Context, cmd command.MarkReadCommand) error {
	if err := requireActor(cmd.ActorID); err != nil {
		return err
	}
	if cmd.NotificationID == uuid.Nil {
		return domain.ErrInvalidNotificationID
	}

	notification, err := s.notifications.FindByID(ctx, cmd.NotificationID)
	if err != nil {
		return fmt.Errorf("find notification: %w", err)
	}
	if notification.UserID != cmd.ActorID {
		return domain.ErrNotificationNotFound
	}
	if err := notification.MarkRead(s.now().UTC()); err != nil {
		return err
	}
	if notification.ReadAt == nil {
		return nil
	}
	if err := s.notifications.MarkRead(ctx, notification.ID, *notification.ReadAt); err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}

	return nil
}

func (s *NotificationService) MarkAllRead(ctx context.Context, cmd command.MarkAllReadCommand) error {
	if err := requireActor(cmd.ActorID); err != nil {
		return err
	}

	if err := s.notifications.MarkAllRead(ctx, cmd.ActorID, s.now().UTC()); err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}

	return nil
}

func requireActor(actorID uuid.UUID) error {
	if actorID == uuid.Nil {
		return application.ErrInvalidActor
	}

	return nil
}

func (s *NotificationService) createTelegramDeliveryIfLinked(ctx context.Context, notification domain.Notification, now time.Time) error {
	if s.telegramLinks == nil || s.deliveries == nil {
		return nil
	}

	link, err := s.telegramLinks.FindByUserID(ctx, notification.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrTelegramLinkNotFound) {
			return nil
		}
		return fmt.Errorf("find telegram link: %w", err)
	}
	if !link.Enabled {
		return nil
	}

	delivery, err := domain.NewNotificationDelivery(uuid.New(), notification.ID, domain.DeliveryChannelTelegram, now)
	if err != nil {
		return err
	}
	if _, err := s.deliveries.Create(ctx, delivery); err != nil {
		return fmt.Errorf("create telegram delivery: %w", err)
	}

	return nil
}

func generateTelegramLinkToken() (string, string, error) {
	bytes := make([]byte, telegramLinkTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generate telegram link token: %w", err)
	}

	rawToken := base64.RawURLEncoding.EncodeToString(bytes)
	return rawToken, hashTelegramLinkToken(rawToken), nil
}

func hashTelegramLinkToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}
