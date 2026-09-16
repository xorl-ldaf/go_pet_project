package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"go_pet_project/internal/notification/application/command"
	outboxdomain "go_pet_project/internal/outbox/domain"

	"github.com/google/uuid"
)

type envelope struct {
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	Version    int             `json:"version"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

type notificationRequestedPayload struct {
	ReminderID      string    `json:"reminder_id"`
	TaskID          string    `json:"task_id"`
	RecipientUserID string    `json:"recipient_user_id"`
	TriggerAt       time.Time `json:"trigger_at"`
}

func decodeNotificationRequested(value []byte) (command.HandleNotificationRequestedCommand, error) {
	var message envelope
	if err := json.Unmarshal(value, &message); err != nil {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("decode envelope: %w", err)
	}
	if message.EventType != outboxdomain.NotificationRequestedV1Type {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("unsupported event type: %s", message.EventType)
	}
	if message.Version != outboxdomain.EventVersion {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("unsupported event version: %d", message.Version)
	}
	eventID, err := uuid.Parse(message.EventID)
	if err != nil {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("parse event id: %w", err)
	}
	if message.OccurredAt.IsZero() {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("occurred_at is required")
	}

	var payload notificationRequestedPayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("decode payload: %w", err)
	}
	recipientID, err := uuid.Parse(payload.RecipientUserID)
	if err != nil {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("parse recipient user id: %w", err)
	}
	taskID, err := uuid.Parse(payload.TaskID)
	if err != nil {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("parse task id: %w", err)
	}
	reminderID, err := uuid.Parse(payload.ReminderID)
	if err != nil {
		return command.HandleNotificationRequestedCommand{}, fmt.Errorf("parse reminder id: %w", err)
	}

	return command.HandleNotificationRequestedCommand{
		EventID:         eventID,
		RecipientUserID: recipientID,
		TaskID:          taskID,
		ReminderID:      reminderID,
		OccurredAt:      message.OccurredAt,
	}, nil
}
