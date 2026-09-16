package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Reminder struct {
	ID            uuid.UUID
	TaskID        uuid.UUID
	Kind          Kind
	OffsetSeconds *int64
	TriggerAt     time.Time
	State         State
	CreatedAt     time.Time
	SentAt        *time.Time
}

func NewAbsoluteReminder(id uuid.UUID, taskID uuid.UUID, triggerAt time.Time, createdAt time.Time) (Reminder, error) {
	return RestoreReminder(id, taskID, KindAbsolute, nil, triggerAt, InitialState, createdAt, nil)
}

func NewBeforeDeadlineReminder(id uuid.UUID, taskID uuid.UUID, taskDeadline time.Time, offsetSeconds int64, createdAt time.Time) (Reminder, error) {
	if offsetSeconds <= 0 {
		return Reminder{}, ErrInvalidOffsetSeconds
	}

	triggerAt := taskDeadline.Add(-time.Duration(offsetSeconds) * time.Second)
	return RestoreReminder(id, taskID, KindBeforeDeadline, int64Ptr(offsetSeconds), triggerAt, InitialState, createdAt, nil)
}

func RestoreReminder(
	id uuid.UUID,
	taskID uuid.UUID,
	kind Kind,
	offsetSeconds *int64,
	triggerAt time.Time,
	state State,
	createdAt time.Time,
	sentAt *time.Time,
) (Reminder, error) {
	if id == uuid.Nil {
		return Reminder{}, ErrInvalidReminderID
	}
	if taskID == uuid.Nil {
		return Reminder{}, ErrInvalidTaskID
	}
	if !kind.IsValid() {
		return Reminder{}, fmt.Errorf("%w: %s", ErrInvalidKind, kind)
	}
	if err := validateKindFields(kind, offsetSeconds); err != nil {
		return Reminder{}, err
	}
	if triggerAt.IsZero() {
		return Reminder{}, ErrInvalidTriggerAt
	}
	if createdAt.IsZero() {
		return Reminder{}, ErrInvalidCreatedAt
	}
	if !state.IsValid() {
		return Reminder{}, fmt.Errorf("%w: %s", ErrInvalidState, state)
	}
	if state == StateSent && sentAt == nil {
		return Reminder{}, ErrInvalidSentAt
	}
	if state != StateSent && sentAt != nil {
		return Reminder{}, ErrInvalidSentAt
	}
	if sentAt != nil && sentAt.Before(createdAt) {
		return Reminder{}, ErrInvalidSentAt
	}

	return Reminder{
		ID:            id,
		TaskID:        taskID,
		Kind:          kind,
		OffsetSeconds: cloneInt64Ptr(offsetSeconds),
		TriggerAt:     triggerAt,
		State:         state,
		CreatedAt:     createdAt,
		SentAt:        cloneTimePtr(sentAt),
	}, nil
}

func (r *Reminder) RescheduleAbsolute(triggerAt time.Time) error {
	if r.Kind != KindAbsolute {
		return ErrInvalidReminderFields
	}
	if !r.State.IsPending() {
		return ErrInvalidReminderLifecycle
	}
	if triggerAt.IsZero() {
		return ErrInvalidTriggerAt
	}

	r.TriggerAt = triggerAt

	return nil
}

func (r *Reminder) RescheduleBeforeDeadline(taskDeadline time.Time, offsetSeconds int64) error {
	if r.Kind != KindBeforeDeadline {
		return ErrInvalidReminderFields
	}
	if !r.State.IsPending() {
		return ErrInvalidReminderLifecycle
	}
	if taskDeadline.IsZero() {
		return ErrTaskDeadlineRequired
	}
	if offsetSeconds <= 0 {
		return ErrInvalidOffsetSeconds
	}

	r.OffsetSeconds = int64Ptr(offsetSeconds)
	r.TriggerAt = taskDeadline.Add(-time.Duration(offsetSeconds) * time.Second)

	return nil
}

func (r *Reminder) RecalculateForDeadline(taskDeadline time.Time) error {
	if r.Kind != KindBeforeDeadline {
		return nil
	}
	if !r.State.IsPending() {
		return ErrInvalidReminderLifecycle
	}
	if taskDeadline.IsZero() {
		return ErrTaskDeadlineRequired
	}
	if r.OffsetSeconds == nil || *r.OffsetSeconds <= 0 {
		return ErrInvalidOffsetSeconds
	}

	r.TriggerAt = taskDeadline.Add(-time.Duration(*r.OffsetSeconds) * time.Second)

	return nil
}

func (r *Reminder) MarkSent(now time.Time) error {
	if !r.State.IsPending() {
		return ErrInvalidReminderLifecycle
	}
	if now.IsZero() || now.Before(r.CreatedAt) {
		return ErrInvalidSentAt
	}

	r.State = StateSent
	r.SentAt = timePtr(now)

	return nil
}

func (r *Reminder) Cancel() error {
	switch r.State {
	case StatePending:
		r.State = StateCancelled
		return nil
	case StateCancelled:
		return nil
	case StateSent:
		return ErrInvalidReminderLifecycle
	default:
		return ErrInvalidState
	}
}

func validateKindFields(kind Kind, offsetSeconds *int64) error {
	switch kind {
	case KindAbsolute:
		if offsetSeconds != nil {
			return ErrInvalidReminderFields
		}
	case KindBeforeDeadline:
		if offsetSeconds == nil || *offsetSeconds <= 0 {
			return ErrInvalidOffsetSeconds
		}
	default:
		return ErrInvalidKind
	}

	return nil
}

func int64Ptr(value int64) *int64 {
	copied := value
	return &copied
}

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}

	return int64Ptr(*value)
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
