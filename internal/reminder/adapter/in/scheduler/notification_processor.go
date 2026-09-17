package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	outboxout "go_pet_project/internal/outbox/application/port/out"
	outboxdomain "go_pet_project/internal/outbox/domain"
	reminderout "go_pet_project/internal/reminder/application/port/out"
	"go_pet_project/internal/reminder/domain"

	"github.com/google/uuid"
)

type NotificationProcessor struct {
	tasks     reminderout.TaskReader
	reminders reminderout.ReminderRepository
	outbox    outboxout.Repository
	now       func() time.Time
}

func NewNotificationProcessor(
	tasks reminderout.TaskReader,
	reminders reminderout.ReminderRepository,
	outbox outboxout.Repository,
) (*NotificationProcessor, error) {
	if tasks == nil {
		return nil, errors.New("task reader is required")
	}
	if reminders == nil {
		return nil, errors.New("reminder repository is required")
	}
	if outbox == nil {
		return nil, errors.New("outbox repository is required")
	}

	return &NotificationProcessor{
		tasks:     tasks,
		reminders: reminders,
		outbox:    outbox,
		now:       time.Now,
	}, nil
}

func (p *NotificationProcessor) Process(ctx context.Context, reminder domain.Reminder) error {
	now := p.now().UTC()
	task, err := p.tasks.FindByID(ctx, reminder.TaskID)
	if err != nil {
		return fmt.Errorf("load task for reminder notification: %w", err)
	}

	payload, err := json.Marshal(NotificationRequestedPayload{
		ReminderID:      reminder.ID.String(),
		TaskID:          reminder.TaskID.String(),
		RecipientUserID: task.AssigneeID.String(),
		TriggerAt:       reminder.TriggerAt,
	})
	if err != nil {
		return fmt.Errorf("marshal reminder notification payload: %w", err)
	}

	event, err := outboxdomain.NewEvent(
		uuid.New(),
		outboxdomain.ReminderAggregateType,
		reminder.ID,
		outboxdomain.NotificationRequestedV1Type,
		payload,
		now,
	)
	if err != nil {
		return fmt.Errorf("create reminder notification outbox event: %w", err)
	}

	if _, err := p.outbox.Create(ctx, event); err != nil {
		return fmt.Errorf("persist reminder notification outbox event: %w", err)
	}
	if err := reminder.MarkSent(now); err != nil {
		return err
	}
	if _, err := p.reminders.Update(ctx, reminder); err != nil {
		return fmt.Errorf("mark reminder sent: %w", err)
	}

	return nil
}

type NotificationRequestedPayload struct {
	ReminderID      string    `json:"reminder_id"`
	TaskID          string    `json:"task_id"`
	RecipientUserID string    `json:"recipient_user_id"`
	TriggerAt       time.Time `json:"trigger_at"`
}
