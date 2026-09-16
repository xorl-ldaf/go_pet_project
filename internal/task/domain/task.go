package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID          uuid.UUID
	SeriesID    *uuid.UUID
	CreatorID   uuid.UUID
	AssigneeID  uuid.UUID
	Title       string
	Description string
	Status      Status
	DeadlineAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	ArchivedAt  *time.Time
}

func NewTask(creatorID uuid.UUID, assigneeID uuid.UUID, title string, description string, deadlineAt *time.Time, now time.Time) (Task, error) {
	if assigneeID == uuid.Nil {
		assigneeID = creatorID
	}

	return RestoreTask(
		uuid.New(),
		nil,
		creatorID,
		assigneeID,
		title,
		description,
		InitialStatus,
		deadlineAt,
		now,
		now,
		nil,
		nil,
	)
}

func NewTaskOccurrence(seriesID uuid.UUID, creatorID uuid.UUID, assigneeID uuid.UUID, title string, description string, deadlineAt time.Time, now time.Time) (Task, error) {
	return RestoreTask(
		uuid.New(),
		&seriesID,
		creatorID,
		assigneeID,
		title,
		description,
		InitialStatus,
		&deadlineAt,
		now,
		now,
		nil,
		nil,
	)
}

func RestoreTask(
	id uuid.UUID,
	seriesID *uuid.UUID,
	creatorID uuid.UUID,
	assigneeID uuid.UUID,
	title string,
	description string,
	status Status,
	deadlineAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
	completedAt *time.Time,
	archivedAt *time.Time,
) (Task, error) {
	if id == uuid.Nil {
		return Task{}, ErrInvalidTaskID
	}
	if seriesID != nil && *seriesID == uuid.Nil {
		return Task{}, ErrInvalidSeriesID
	}
	if creatorID == uuid.Nil {
		return Task{}, ErrInvalidCreatorID
	}
	if assigneeID == uuid.Nil {
		return Task{}, ErrInvalidAssigneeID
	}
	if err := validateTitle(title); err != nil {
		return Task{}, err
	}
	if !status.IsValid() {
		return Task{}, fmt.Errorf("%w: %s", ErrInvalidStatus, status)
	}
	if createdAt.IsZero() {
		return Task{}, fmt.Errorf("%w: createdAt must not be zero", ErrInvalidTimestamp)
	}
	if err := validateUpdatedAt(createdAt, createdAt, updatedAt); err != nil {
		return Task{}, err
	}
	if status.IsCompleted() && completedAt == nil {
		completedAt = timePtr(updatedAt)
	}
	if !status.IsCompleted() && completedAt != nil {
		return Task{}, fmt.Errorf("%w: completedAt requires completed status", ErrInvalidStatus)
	}

	return Task{
		ID:          id,
		SeriesID:    cloneUUIDPtr(seriesID),
		CreatorID:   creatorID,
		AssigneeID:  assigneeID,
		Title:       title,
		Description: description,
		Status:      status,
		DeadlineAt:  cloneTimePtr(deadlineAt),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		CompletedAt: cloneTimePtr(completedAt),
		ArchivedAt:  cloneTimePtr(archivedAt),
	}, nil
}

func (t *Task) ChangeStatus(next Status, now time.Time) error {
	if !next.IsValid() {
		return fmt.Errorf("%w: %s", ErrInvalidStatus, next)
	}
	if !t.Status.CanTransitionTo(next) {
		return StatusTransitionError{From: t.Status, To: next}
	}
	if err := t.validateMutationTime(now); err != nil {
		return err
	}

	t.Status = next
	t.UpdatedAt = now
	if next.IsCompleted() {
		t.CompletedAt = timePtr(now)
	}

	return nil
}

func (t *Task) UpdateTitle(title string, now time.Time) error {
	if err := validateTitle(title); err != nil {
		return err
	}
	if err := t.validateMutationTime(now); err != nil {
		return err
	}

	t.Title = title
	t.UpdatedAt = now

	return nil
}

func (t *Task) UpdateDescription(description string, now time.Time) error {
	if err := t.validateMutationTime(now); err != nil {
		return err
	}

	t.Description = description
	t.UpdatedAt = now

	return nil
}

func (t *Task) UpdateDeadline(deadlineAt *time.Time, now time.Time) error {
	if err := t.validateMutationTime(now); err != nil {
		return err
	}

	t.DeadlineAt = cloneTimePtr(deadlineAt)
	t.UpdatedAt = now

	return nil
}

func (t *Task) Reassign(assigneeID uuid.UUID, now time.Time) error {
	if assigneeID == uuid.Nil {
		return ErrInvalidAssigneeID
	}
	if assigneeID == t.AssigneeID {
		return nil
	}
	if err := t.validateMutationTime(now); err != nil {
		return err
	}

	t.AssigneeID = assigneeID
	t.UpdatedAt = now

	return nil
}

func (t *Task) Archive(now time.Time) error {
	if t.ArchivedAt != nil {
		return nil
	}
	if err := t.validateMutationTime(now); err != nil {
		return err
	}

	t.ArchivedAt = timePtr(now)
	t.UpdatedAt = now

	return nil
}

func (t *Task) Restore(now time.Time) error {
	if t.ArchivedAt == nil {
		return nil
	}
	if err := t.validateMutationTime(now); err != nil {
		return err
	}

	t.ArchivedAt = nil
	t.UpdatedAt = now

	return nil
}

func (t Task) IsArchived() bool {
	return t.ArchivedAt != nil
}

func (t Task) IsOverdue(now time.Time) bool {
	if t.DeadlineAt == nil || t.Status.IsCompleted() {
		return false
	}

	return t.DeadlineAt.Before(now)
}

func (t Task) validateMutationTime(now time.Time) error {
	return validateUpdatedAt(t.CreatedAt, t.UpdatedAt, now)
}

func validateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return ErrInvalidTitle
	}

	return nil
}

func validateUpdatedAt(createdAt time.Time, currentUpdatedAt time.Time, nextUpdatedAt time.Time) error {
	if nextUpdatedAt.IsZero() {
		return fmt.Errorf("%w: updatedAt must not be zero", ErrInvalidTimestamp)
	}
	if nextUpdatedAt.Before(createdAt) {
		return fmt.Errorf("%w: updatedAt must not be before createdAt", ErrInvalidTimestamp)
	}
	if nextUpdatedAt.Before(currentUpdatedAt) {
		return fmt.Errorf("%w: updatedAt must not move backwards", ErrInvalidTimestamp)
	}

	return nil
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
