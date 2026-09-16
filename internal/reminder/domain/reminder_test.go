package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewAbsoluteReminder(t *testing.T) {
	taskID := uuid.New()
	triggerAt := time.Date(2026, 9, 20, 16, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	reminder, err := NewAbsoluteReminder(uuid.New(), taskID, triggerAt, createdAt)
	if err != nil {
		t.Fatalf("NewAbsoluteReminder: %v", err)
	}

	if reminder.TaskID != taskID ||
		reminder.Kind != KindAbsolute ||
		reminder.OffsetSeconds != nil ||
		!reminder.TriggerAt.Equal(triggerAt) ||
		reminder.State != StatePending ||
		!reminder.CreatedAt.Equal(createdAt) ||
		reminder.SentAt != nil {
		t.Fatalf("unexpected absolute reminder: %#v", reminder)
	}
}

func TestNewBeforeDeadlineReminderCalculatesTrigger(t *testing.T) {
	deadline := time.Date(2026, 9, 20, 16, 0, 0, 0, time.UTC)

	reminder, err := NewBeforeDeadlineReminder(uuid.New(), uuid.New(), deadline, 7200, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewBeforeDeadlineReminder: %v", err)
	}

	if reminder.Kind != KindBeforeDeadline ||
		reminder.OffsetSeconds == nil ||
		*reminder.OffsetSeconds != 7200 ||
		!reminder.TriggerAt.Equal(deadline.Add(-2*time.Hour)) ||
		reminder.State != StatePending {
		t.Fatalf("unexpected relative reminder: %#v", reminder)
	}
}

func TestNewBeforeDeadlineReminderInvalidOffset(t *testing.T) {
	_, err := NewBeforeDeadlineReminder(uuid.New(), uuid.New(), time.Now().UTC(), 0, time.Now().UTC())
	if !errors.Is(err, ErrInvalidOffsetSeconds) {
		t.Fatalf("error = %v, want ErrInvalidOffsetSeconds", err)
	}
}

func TestRecalculateForDeadlinePendingRelativeOnly(t *testing.T) {
	createdAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	deadline := time.Date(2026, 9, 20, 16, 0, 0, 0, time.UTC)
	reminder, err := NewBeforeDeadlineReminder(uuid.New(), uuid.New(), deadline, 3600, createdAt)
	if err != nil {
		t.Fatalf("NewBeforeDeadlineReminder: %v", err)
	}

	nextDeadline := deadline.Add(24 * time.Hour)
	if err := reminder.RecalculateForDeadline(nextDeadline); err != nil {
		t.Fatalf("RecalculateForDeadline: %v", err)
	}
	if !reminder.TriggerAt.Equal(nextDeadline.Add(-time.Hour)) {
		t.Fatalf("TriggerAt = %s, want %s", reminder.TriggerAt, nextDeadline.Add(-time.Hour))
	}

	if err := reminder.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	previousTrigger := reminder.TriggerAt
	err = reminder.RecalculateForDeadline(nextDeadline.Add(24 * time.Hour))
	if !errors.Is(err, ErrInvalidReminderLifecycle) {
		t.Fatalf("cancelled recalculate error = %v, want ErrInvalidReminderLifecycle", err)
	}
	if !reminder.TriggerAt.Equal(previousTrigger) {
		t.Fatalf("cancelled reminder TriggerAt changed")
	}
}

func TestStateTransitions(t *testing.T) {
	createdAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	reminder, err := NewAbsoluteReminder(uuid.New(), uuid.New(), createdAt.Add(time.Hour), createdAt)
	if err != nil {
		t.Fatalf("NewAbsoluteReminder: %v", err)
	}

	sentAt := createdAt.Add(30 * time.Minute)
	if err := reminder.MarkSent(sentAt); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if reminder.State != StateSent || reminder.SentAt == nil || !reminder.SentAt.Equal(sentAt) {
		t.Fatalf("unexpected sent reminder: %#v", reminder)
	}
	if err := reminder.Cancel(); !errors.Is(err, ErrInvalidReminderLifecycle) {
		t.Fatalf("Cancel sent error = %v, want ErrInvalidReminderLifecycle", err)
	}

	cancelled, err := NewAbsoluteReminder(uuid.New(), uuid.New(), createdAt.Add(time.Hour), createdAt)
	if err != nil {
		t.Fatalf("NewAbsoluteReminder: %v", err)
	}
	if err := cancelled.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.State != StateCancelled {
		t.Fatalf("State = %s, want CANCELLED", cancelled.State)
	}
	if err := cancelled.MarkSent(sentAt); !errors.Is(err, ErrInvalidReminderLifecycle) {
		t.Fatalf("MarkSent cancelled error = %v, want ErrInvalidReminderLifecycle", err)
	}
}
