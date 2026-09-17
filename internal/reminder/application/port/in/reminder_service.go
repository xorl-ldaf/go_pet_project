package in

import (
	"context"

	"go_pet_project/internal/reminder/application/command"
	"go_pet_project/internal/reminder/application/query"
	"go_pet_project/internal/reminder/domain"
)

type ReminderService interface {
	CreateReminder(ctx context.Context, cmd command.CreateReminderCommand) (domain.Reminder, error)
	ListReminders(ctx context.Context, q query.ListRemindersQuery) ([]domain.Reminder, error)
	UpdateReminder(ctx context.Context, cmd command.UpdateReminderCommand) (domain.Reminder, error)
	DeleteReminder(ctx context.Context, cmd command.DeleteReminderCommand) error
}
