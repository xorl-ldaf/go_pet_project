package postgres

import (
	"fmt"
	"time"

	"go_pet_project/internal/reminder/domain"
)

func toModel(reminder domain.Reminder) reminderModel {
	return reminderModel{
		ID:            reminder.ID,
		TaskID:        reminder.TaskID,
		Kind:          string(reminder.Kind),
		OffsetSeconds: cloneInt64Ptr(reminder.OffsetSeconds),
		TriggerAt:     reminder.TriggerAt,
		State:         string(reminder.State),
		CreatedAt:     reminder.CreatedAt,
		SentAt:        cloneTimePtr(reminder.SentAt),
	}
}

func toDomain(model reminderModel) (domain.Reminder, error) {
	kind, err := domain.ParseKind(model.Kind)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("map reminder model to domain: %w", err)
	}
	state, err := domain.ParseState(model.State)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("map reminder model to domain: %w", err)
	}

	reminder, err := domain.RestoreReminder(
		model.ID,
		model.TaskID,
		kind,
		cloneInt64Ptr(model.OffsetSeconds),
		model.TriggerAt,
		state,
		model.CreatedAt,
		cloneTimePtr(model.SentAt),
	)
	if err != nil {
		return domain.Reminder{}, fmt.Errorf("map reminder model to domain: %w", err)
	}

	return reminder, nil
}

func cloneInt64Ptr(value *int64) *int64 {
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
