package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TaskID     *uuid.UUID
	ReminderID *uuid.UUID
	Type       Type
	Title      string
	Body       string
	CreatedAt  time.Time
	ReadAt     *time.Time
}

func NewNotification(id uuid.UUID, userID uuid.UUID, taskID *uuid.UUID, reminderID *uuid.UUID, notificationType Type, title string, body string, createdAt time.Time) (Notification, error) {
	return RestoreNotification(id, userID, taskID, reminderID, notificationType, title, body, createdAt, nil)
}

func RestoreNotification(
	id uuid.UUID,
	userID uuid.UUID,
	taskID *uuid.UUID,
	reminderID *uuid.UUID,
	notificationType Type,
	title string,
	body string,
	createdAt time.Time,
	readAt *time.Time,
) (Notification, error) {
	if id == uuid.Nil {
		return Notification{}, ErrInvalidNotificationID
	}
	if userID == uuid.Nil {
		return Notification{}, ErrInvalidUserID
	}
	if taskID != nil && *taskID == uuid.Nil {
		return Notification{}, ErrInvalidTaskID
	}
	if reminderID != nil && *reminderID == uuid.Nil {
		return Notification{}, ErrInvalidReminderID
	}
	if !notificationType.IsValid() {
		return Notification{}, fmt.Errorf("%w: %s", ErrInvalidType, notificationType)
	}
	if strings.TrimSpace(title) == "" {
		return Notification{}, ErrInvalidTitle
	}
	if strings.TrimSpace(body) == "" {
		return Notification{}, ErrInvalidBody
	}
	if createdAt.IsZero() {
		return Notification{}, ErrInvalidCreatedAt
	}
	if readAt != nil && readAt.Before(createdAt) {
		return Notification{}, ErrInvalidReadAt
	}

	return Notification{
		ID:         id,
		UserID:     userID,
		TaskID:     cloneUUIDPtr(taskID),
		ReminderID: cloneUUIDPtr(reminderID),
		Type:       notificationType,
		Title:      title,
		Body:       body,
		CreatedAt:  createdAt,
		ReadAt:     cloneTimePtr(readAt),
	}, nil
}

func (n *Notification) MarkRead(now time.Time) error {
	if n.ReadAt != nil {
		return nil
	}
	if now.IsZero() || now.Before(n.CreatedAt) {
		return ErrInvalidReadAt
	}
	n.ReadAt = timePtr(now)

	return nil
}

func (n Notification) IsUnread() bool {
	return n.ReadAt == nil
}

func timePtr(value time.Time) *time.Time {
	copied := value
	return &copied
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	return timePtr(*value)
}

func cloneUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}
