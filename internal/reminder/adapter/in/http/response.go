package httpadapter

import (
	"time"

	"go_pet_project/internal/platform/httpx"
	"go_pet_project/internal/reminder/domain"
)

type ReminderResponse struct {
	ID            string     `json:"id"`
	TaskID        string     `json:"task_id"`
	Kind          string     `json:"kind"`
	OffsetSeconds *int64     `json:"offset_seconds"`
	TriggerAt     time.Time  `json:"trigger_at"`
	State         string     `json:"state"`
	CreatedAt     time.Time  `json:"created_at"`
	SentAt        *time.Time `json:"sent_at"`
}

type ReminderListResponse struct {
	Items []ReminderResponse `json:"items"`
}

func newReminderResponse(reminder domain.Reminder) ReminderResponse {
	return ReminderResponse{
		ID:            reminder.ID.String(),
		TaskID:        reminder.TaskID.String(),
		Kind:          string(reminder.Kind),
		OffsetSeconds: cloneInt64Ptr(reminder.OffsetSeconds),
		TriggerAt:     reminder.TriggerAt,
		State:         string(reminder.State),
		CreatedAt:     reminder.CreatedAt,
		SentAt:        cloneTimePtr(reminder.SentAt),
	}
}

func newReminderListResponse(reminders []domain.Reminder) ReminderListResponse {
	items := make([]ReminderResponse, 0, len(reminders))
	for _, reminder := range reminders {
		items = append(items, newReminderResponse(reminder))
	}

	return ReminderListResponse{Items: items}
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

type ErrorResponse = httpx.ErrorResponse
type ErrorBody = httpx.ErrorBody
