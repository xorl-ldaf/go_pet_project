package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type DeliveryChannel string

const DeliveryChannelTelegram DeliveryChannel = "TELEGRAM"

func ParseDeliveryChannel(value string) (DeliveryChannel, error) {
	channel := DeliveryChannel(value)
	if !channel.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidDeliveryChannel, value)
	}

	return channel, nil
}

func (c DeliveryChannel) IsValid() bool {
	return c == DeliveryChannelTelegram
}

type DeliveryStatus string

const (
	DeliveryStatusPending DeliveryStatus = "PENDING"
	DeliveryStatusSent    DeliveryStatus = "SENT"
	DeliveryStatusFailed  DeliveryStatus = "FAILED"
)

func ParseDeliveryStatus(value string) (DeliveryStatus, error) {
	status := DeliveryStatus(value)
	if !status.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidDeliveryStatus, value)
	}

	return status, nil
}

func (s DeliveryStatus) IsValid() bool {
	return s == DeliveryStatusPending || s == DeliveryStatusSent || s == DeliveryStatusFailed
}

type NotificationDelivery struct {
	ID             uuid.UUID
	NotificationID uuid.UUID
	Channel        DeliveryChannel
	Status         DeliveryStatus
	Attempts       int
	NextAttemptAt  *time.Time
	SentAt         *time.Time
	LastError      *string
}

func NewNotificationDelivery(id uuid.UUID, notificationID uuid.UUID, channel DeliveryChannel, now time.Time) (NotificationDelivery, error) {
	return RestoreNotificationDelivery(id, notificationID, channel, DeliveryStatusPending, 0, &now, nil, nil)
}

func RestoreNotificationDelivery(
	id uuid.UUID,
	notificationID uuid.UUID,
	channel DeliveryChannel,
	status DeliveryStatus,
	attempts int,
	nextAttemptAt *time.Time,
	sentAt *time.Time,
	lastError *string,
) (NotificationDelivery, error) {
	if id == uuid.Nil {
		return NotificationDelivery{}, ErrInvalidDeliveryID
	}
	if notificationID == uuid.Nil {
		return NotificationDelivery{}, ErrInvalidNotificationID
	}
	if !channel.IsValid() {
		return NotificationDelivery{}, ErrInvalidDeliveryChannel
	}
	if !status.IsValid() {
		return NotificationDelivery{}, ErrInvalidDeliveryStatus
	}
	if attempts < 0 {
		return NotificationDelivery{}, ErrInvalidDeliveryAttempts
	}
	if nextAttemptAt != nil && nextAttemptAt.IsZero() {
		return NotificationDelivery{}, ErrInvalidDeliveryNextAttempt
	}
	if sentAt != nil && sentAt.IsZero() {
		return NotificationDelivery{}, ErrInvalidDeliverySentAt
	}
	if lastError != nil {
		trimmed := strings.TrimSpace(*lastError)
		if trimmed == "" {
			lastError = nil
		} else {
			lastError = &trimmed
		}
	}

	return NotificationDelivery{
		ID:             id,
		NotificationID: notificationID,
		Channel:        channel,
		Status:         status,
		Attempts:       attempts,
		NextAttemptAt:  cloneTimePtr(nextAttemptAt),
		SentAt:         cloneTimePtr(sentAt),
		LastError:      cloneStringPtr(lastError),
	}, nil
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}
