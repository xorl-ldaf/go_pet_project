package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNotificationMarkRead(t *testing.T) {
	createdAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	notification, err := NewNotification(uuid.New(), uuid.New(), nil, nil, TypeTaskReminder, "Task reminder", "Reminder is due.", createdAt)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}

	readAt := createdAt.Add(time.Hour)
	if err := notification.MarkRead(readAt); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if notification.ReadAt == nil || !notification.ReadAt.Equal(readAt) {
		t.Fatalf("ReadAt = %v, want %s", notification.ReadAt, readAt)
	}
}

func TestNotificationMarkReadIdempotent(t *testing.T) {
	createdAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	readAt := createdAt.Add(time.Hour)
	notification, err := RestoreNotification(uuid.New(), uuid.New(), nil, nil, TypeTaskReminder, "Task reminder", "Reminder is due.", createdAt, &readAt)
	if err != nil {
		t.Fatalf("RestoreNotification: %v", err)
	}

	nextReadAt := readAt.Add(time.Hour)
	if err := notification.MarkRead(nextReadAt); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if notification.ReadAt == nil || !notification.ReadAt.Equal(readAt) {
		t.Fatalf("ReadAt = %v, want unchanged %s", notification.ReadAt, readAt)
	}
}
