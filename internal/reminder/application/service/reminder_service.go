package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go_pet_project/internal/reminder/application"
	"go_pet_project/internal/reminder/application/command"
	reminderin "go_pet_project/internal/reminder/application/port/in"
	reminderout "go_pet_project/internal/reminder/application/port/out"
	reminderquery "go_pet_project/internal/reminder/application/query"
	"go_pet_project/internal/reminder/domain"
	taskdomain "go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

var _ reminderin.ReminderService = (*ReminderService)(nil)

type ReminderService struct {
	tasks     reminderout.TaskReader
	reminders reminderout.ReminderRepository
	now       func() time.Time
}

func NewReminderService(tasks reminderout.TaskReader, reminders reminderout.ReminderRepository) (*ReminderService, error) {
	if tasks == nil {
		return nil, errors.New("task reader is required")
	}
	if reminders == nil {
		return nil, errors.New("reminder repository is required")
	}

	return &ReminderService{
		tasks:     tasks,
		reminders: reminders,
		now:       time.Now,
	}, nil
}

func (s *ReminderService) CreateReminder(ctx context.Context, cmd command.CreateReminderCommand) (domain.Reminder, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return domain.Reminder{}, err
	}

	task, err := s.tasks.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("load task for reminder create: %w", err)
	}
	if cmd.ActorID != task.CreatorID {
		return domain.Reminder{}, application.ErrReminderAccessDenied
	}
	if task.Status.IsCompleted() {
		return domain.Reminder{}, domain.ErrTaskAlreadyCompleted
	}

	now := s.now().UTC()
	reminder, err := s.buildNewReminder(cmd, task, now)
	if err != nil {
		return domain.Reminder{}, err
	}
	if !reminder.TriggerAt.After(now) {
		return domain.Reminder{}, domain.ErrInvalidTriggerAt
	}

	created, err := s.reminders.Create(ctx, reminder)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("create reminder: %w", err)
	}

	return created, nil
}

func (s *ReminderService) ListReminders(ctx context.Context, q reminderquery.ListRemindersQuery) ([]domain.Reminder, error) {
	if err := requireActor(q.ActorID); err != nil {
		return nil, err
	}

	task, err := s.tasks.FindByID(ctx, q.TaskID)
	if err != nil {
		return nil, fmt.Errorf("load task for reminder list: %w", err)
	}
	if !canRead(q.ActorID, task) {
		return nil, application.ErrReminderAccessDenied
	}

	reminders, err := s.reminders.ListByTaskID(ctx, q.TaskID)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}

	return reminders, nil
}

func (s *ReminderService) UpdateReminder(ctx context.Context, cmd command.UpdateReminderCommand) (domain.Reminder, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return domain.Reminder{}, err
	}

	task, err := s.tasks.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("load task for reminder update: %w", err)
	}
	if cmd.ActorID != task.CreatorID {
		return domain.Reminder{}, application.ErrReminderAccessDenied
	}

	reminder, err := s.reminders.FindByID(ctx, cmd.ReminderID)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("find reminder: %w", err)
	}
	if reminder.TaskID != cmd.TaskID {
		return domain.Reminder{}, domain.ErrReminderNotFound
	}

	if err := s.applyUpdate(&reminder, task, cmd); err != nil {
		return domain.Reminder{}, err
	}
	if !reminder.TriggerAt.After(s.now().UTC()) {
		return domain.Reminder{}, domain.ErrInvalidTriggerAt
	}

	updated, err := s.reminders.Update(ctx, reminder)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("update reminder: %w", err)
	}

	return updated, nil
}

func (s *ReminderService) DeleteReminder(ctx context.Context, cmd command.DeleteReminderCommand) error {
	if err := requireActor(cmd.ActorID); err != nil {
		return err
	}

	task, err := s.tasks.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return fmt.Errorf("load task for reminder delete: %w", err)
	}
	if cmd.ActorID != task.CreatorID {
		return application.ErrReminderAccessDenied
	}

	reminder, err := s.reminders.FindByID(ctx, cmd.ReminderID)
	if err != nil {
		return fmt.Errorf("find reminder: %w", err)
	}
	if reminder.TaskID != cmd.TaskID {
		return domain.ErrReminderNotFound
	}

	if err := s.reminders.Delete(ctx, cmd.ReminderID); err != nil {
		return fmt.Errorf("delete reminder: %w", err)
	}

	return nil
}

func (s *ReminderService) RecalculatePendingBeforeDeadline(ctx context.Context, taskID uuid.UUID, deadlineAt *time.Time) error {
	if taskID == uuid.Nil {
		return domain.ErrInvalidTaskID
	}
	if deadlineAt == nil {
		hasRelative, err := s.reminders.HasPendingBeforeDeadline(ctx, taskID)
		if err != nil {
			return fmt.Errorf("check pending relative reminders: %w", err)
		}
		if hasRelative {
			return domain.ErrTaskDeadlineRequired
		}

		return nil
	}

	if err := s.reminders.RecalculatePendingBeforeDeadline(ctx, taskID, *deadlineAt); err != nil {
		return fmt.Errorf("recalculate pending relative reminders: %w", err)
	}

	return nil
}

func (s *ReminderService) CancelPending(ctx context.Context, taskID uuid.UUID) error {
	if taskID == uuid.Nil {
		return domain.ErrInvalidTaskID
	}

	if err := s.reminders.CancelPendingByTaskID(ctx, taskID); err != nil {
		return fmt.Errorf("cancel pending reminders: %w", err)
	}

	return nil
}

func (s *ReminderService) buildNewReminder(cmd command.CreateReminderCommand, task taskdomain.Task, now time.Time) (domain.Reminder, error) {
	switch cmd.Kind {
	case domain.KindAbsolute:
		if cmd.TriggerAt == nil || cmd.OffsetSeconds != nil {
			return domain.Reminder{}, domain.ErrInvalidReminderFields
		}

		return domain.NewAbsoluteReminder(uuid.New(), task.ID, cmd.TriggerAt.UTC(), now)
	case domain.KindBeforeDeadline:
		if cmd.OffsetSeconds == nil || cmd.TriggerAt != nil {
			return domain.Reminder{}, domain.ErrInvalidReminderFields
		}
		if task.DeadlineAt == nil {
			return domain.Reminder{}, domain.ErrTaskDeadlineRequired
		}

		return domain.NewBeforeDeadlineReminder(uuid.New(), task.ID, task.DeadlineAt.UTC(), *cmd.OffsetSeconds, now)
	default:
		return domain.Reminder{}, domain.ErrInvalidKind
	}
}

func (s *ReminderService) applyUpdate(reminder *domain.Reminder, task taskdomain.Task, cmd command.UpdateReminderCommand) error {
	switch reminder.Kind {
	case domain.KindAbsolute:
		if cmd.OffsetSeconds != nil || cmd.TriggerAt == nil {
			return domain.ErrInvalidReminderFields
		}

		return reminder.RescheduleAbsolute(cmd.TriggerAt.UTC())
	case domain.KindBeforeDeadline:
		if cmd.TriggerAt != nil || cmd.OffsetSeconds == nil {
			return domain.ErrInvalidReminderFields
		}
		if task.DeadlineAt == nil {
			return domain.ErrTaskDeadlineRequired
		}

		return reminder.RescheduleBeforeDeadline(task.DeadlineAt.UTC(), *cmd.OffsetSeconds)
	default:
		return domain.ErrInvalidKind
	}
}

func requireActor(actorID uuid.UUID) error {
	if actorID == uuid.Nil {
		return application.ErrInvalidActor
	}

	return nil
}

func canRead(actorID uuid.UUID, task taskdomain.Task) bool {
	return actorID == task.CreatorID || actorID == task.AssigneeID
}
