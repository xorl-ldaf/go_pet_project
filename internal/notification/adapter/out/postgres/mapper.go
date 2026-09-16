package postgres

import (
	"fmt"
	"time"

	"go_pet_project/internal/notification/domain"

	"github.com/google/uuid"
)

func toNotificationModel(notification domain.Notification) notificationModel {
	return notificationModel{
		ID:         notification.ID,
		UserID:     notification.UserID,
		TaskID:     cloneUUIDPtr(notification.TaskID),
		ReminderID: cloneUUIDPtr(notification.ReminderID),
		Type:       string(notification.Type),
		Title:      notification.Title,
		Body:       notification.Body,
		CreatedAt:  notification.CreatedAt,
		ReadAt:     cloneTimePtr(notification.ReadAt),
	}
}

func toNotificationDomain(model notificationModel) (domain.Notification, error) {
	notificationType, err := domain.ParseType(model.Type)
	if err != nil {
		return domain.Notification{}, fmt.Errorf("map notification model to domain: %w", err)
	}

	notification, err := domain.RestoreNotification(
		model.ID,
		model.UserID,
		cloneUUIDPtr(model.TaskID),
		cloneUUIDPtr(model.ReminderID),
		notificationType,
		model.Title,
		model.Body,
		model.CreatedAt,
		cloneTimePtr(model.ReadAt),
	)
	if err != nil {
		return domain.Notification{}, fmt.Errorf("map notification model to domain: %w", err)
	}

	return notification, nil
}

func cloneUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}

func toTelegramLinkModel(link domain.TelegramLink) telegramLinkModel {
	return telegramLinkModel{
		UserID:           link.UserID,
		ChatID:           link.ChatID,
		TelegramUsername: cloneStringPtr(link.TelegramUsername),
		LinkedAt:         link.LinkedAt,
		Enabled:          link.Enabled,
	}
}

func toTelegramLinkDomain(model telegramLinkModel) (domain.TelegramLink, error) {
	link, err := domain.RestoreTelegramLink(
		model.UserID,
		model.ChatID,
		cloneStringPtr(model.TelegramUsername),
		model.LinkedAt,
		model.Enabled,
	)
	if err != nil {
		return domain.TelegramLink{}, fmt.Errorf("map telegram link model to domain: %w", err)
	}

	return link, nil
}

func toTelegramLinkTokenModel(token domain.TelegramLinkToken) telegramLinkTokenModel {
	return telegramLinkTokenModel{
		ID:        token.ID,
		UserID:    token.UserID,
		TokenHash: token.TokenHash,
		ExpiresAt: token.ExpiresAt,
		UsedAt:    cloneTimePtr(token.UsedAt),
		CreatedAt: token.CreatedAt,
	}
}

func toTelegramLinkTokenDomain(model telegramLinkTokenModel) (domain.TelegramLinkToken, error) {
	token, err := domain.RestoreTelegramLinkToken(
		model.ID,
		model.UserID,
		model.TokenHash,
		model.ExpiresAt,
		cloneTimePtr(model.UsedAt),
		model.CreatedAt,
	)
	if err != nil {
		return domain.TelegramLinkToken{}, fmt.Errorf("map telegram link token model to domain: %w", err)
	}

	return token, nil
}

func toNotificationDeliveryModel(delivery domain.NotificationDelivery) notificationDeliveryModel {
	return notificationDeliveryModel{
		ID:             delivery.ID,
		NotificationID: delivery.NotificationID,
		Channel:        string(delivery.Channel),
		Status:         string(delivery.Status),
		Attempts:       delivery.Attempts,
		NextAttemptAt:  cloneTimePtr(delivery.NextAttemptAt),
		SentAt:         cloneTimePtr(delivery.SentAt),
		LastError:      cloneStringPtr(delivery.LastError),
	}
}

func toNotificationDeliveryDomain(model notificationDeliveryModel) (domain.NotificationDelivery, error) {
	channel, err := domain.ParseDeliveryChannel(model.Channel)
	if err != nil {
		return domain.NotificationDelivery{}, fmt.Errorf("map delivery model to domain: %w", err)
	}
	status, err := domain.ParseDeliveryStatus(model.Status)
	if err != nil {
		return domain.NotificationDelivery{}, fmt.Errorf("map delivery model to domain: %w", err)
	}

	delivery, err := domain.RestoreNotificationDelivery(
		model.ID,
		model.NotificationID,
		channel,
		status,
		model.Attempts,
		cloneTimePtr(model.NextAttemptAt),
		cloneTimePtr(model.SentAt),
		cloneStringPtr(model.LastError),
	)
	if err != nil {
		return domain.NotificationDelivery{}, fmt.Errorf("map delivery model to domain: %w", err)
	}

	return delivery, nil
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}
