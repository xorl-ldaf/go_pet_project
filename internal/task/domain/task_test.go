package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewTaskInitialState(t *testing.T) {
	now := testTime()
	creatorID := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	assigneeID := uuid.MustParse("10000000-0000-4000-8000-000000000002")
	deadline := now.Add(24 * time.Hour)

	task, err := NewTask(creatorID, assigneeID, "Ship Stage 8", "", &deadline, now)
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	if task.ID == uuid.Nil {
		t.Fatalf("task id must be generated")
	}
	if task.CreatorID != creatorID || task.AssigneeID != assigneeID {
		t.Fatalf("unexpected identities: %#v", task)
	}
	if InitialStatus != StatusOpen {
		t.Fatalf("InitialStatus = %s, want OPEN", InitialStatus)
	}
	if task.Status != StatusOpen {
		t.Fatalf("status = %s, want OPEN", task.Status)
	}
	if task.ArchivedAt != nil {
		t.Fatalf("new task must not be archived")
	}
	if task.CompletedAt != nil {
		t.Fatalf("new OPEN task must not have CompletedAt")
	}
	if task.DeadlineAt == nil || !task.DeadlineAt.Equal(deadline) {
		t.Fatalf("deadline = %v, want %s", task.DeadlineAt, deadline)
	}
}

func TestNewTaskAllowsCreatorAsAssignee(t *testing.T) {
	now := testTime()
	creatorID := uuid.MustParse("10000000-0000-4000-8000-000000000001")

	task, err := NewTask(creatorID, creatorID, "Self assigned", "", nil, now)
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	if task.CreatorID != creatorID || task.AssigneeID != creatorID {
		t.Fatalf("creator == assignee must be allowed: %#v", task)
	}
}

func TestNewTaskDefaultsNilAssigneeToCreator(t *testing.T) {
	now := testTime()
	creatorID := uuid.MustParse("10000000-0000-4000-8000-000000000001")

	task, err := NewTask(creatorID, uuid.Nil, "Self assigned", "", nil, now)
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	if task.AssigneeID != creatorID {
		t.Fatalf("assignee = %s, want creator %s", task.AssigneeID, creatorID)
	}
}

func TestChangeStatusAllowedTransitions(t *testing.T) {
	tests := []struct {
		name          string
		from          Status
		to            Status
		wantCompleted bool
	}{
		{name: "open to in progress", from: StatusOpen, to: StatusInProgress},
		{name: "open to done", from: StatusOpen, to: StatusDone, wantCompleted: true},
		{name: "in progress to done", from: StatusInProgress, to: StatusDone, wantCompleted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := testTime()
			task := restoredTask(t, tt.from, now, now)
			changedAt := now.Add(time.Minute)

			if err := task.ChangeStatus(tt.to, changedAt); err != nil {
				t.Fatalf("ChangeStatus: %v", err)
			}

			if task.Status != tt.to {
				t.Fatalf("status = %s, want %s", task.Status, tt.to)
			}
			if !task.UpdatedAt.Equal(changedAt) {
				t.Fatalf("UpdatedAt = %s, want %s", task.UpdatedAt, changedAt)
			}
			if tt.wantCompleted {
				if task.CompletedAt == nil || !task.CompletedAt.Equal(changedAt) {
					t.Fatalf("CompletedAt = %v, want %s", task.CompletedAt, changedAt)
				}
			} else if task.CompletedAt != nil {
				t.Fatalf("CompletedAt = %v, want nil", task.CompletedAt)
			}
		})
	}
}

func TestChangeStatusRejectsInvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
	}{
		{name: "open to cancelled", from: StatusOpen, to: StatusCancelled},
		{name: "in progress to open", from: StatusInProgress, to: StatusOpen},
		{name: "in progress to cancelled", from: StatusInProgress, to: StatusCancelled},
		{name: "done to open", from: StatusDone, to: StatusOpen},
		{name: "done to in progress", from: StatusDone, to: StatusInProgress},
		{name: "done to cancelled", from: StatusDone, to: StatusCancelled},
		{name: "cancelled to open", from: StatusCancelled, to: StatusOpen},
		{name: "cancelled to in progress", from: StatusCancelled, to: StatusInProgress},
		{name: "cancelled to done", from: StatusCancelled, to: StatusDone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := testTime()
			task := restoredTask(t, tt.from, now, now)

			err := task.ChangeStatus(tt.to, now.Add(time.Minute))
			if !errors.Is(err, ErrInvalidStatusTransition) {
				t.Fatalf("ChangeStatus error = %v, want ErrInvalidStatusTransition", err)
			}

			var transitionErr StatusTransitionError
			if !errors.As(err, &transitionErr) {
				t.Fatalf("ChangeStatus error %T, want StatusTransitionError", err)
			}
			if transitionErr.From != tt.from || transitionErr.To != tt.to {
				t.Fatalf("transition error = %s -> %s, want %s -> %s", transitionErr.From, transitionErr.To, tt.from, tt.to)
			}
		})
	}
}

func TestChangeStatusRejectsInvalidStatus(t *testing.T) {
	now := testTime()
	task := restoredTask(t, StatusOpen, now, now)

	err := task.ChangeStatus(Status("ANYTHING"), now.Add(time.Minute))
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("ChangeStatus error = %v, want ErrInvalidStatus", err)
	}
}

func TestTitleValidation(t *testing.T) {
	now := testTime()

	if _, err := NewTask(uuid.New(), uuid.New(), "", "", nil, now); !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("empty title error = %v, want ErrInvalidTitle", err)
	}
	if _, err := NewTask(uuid.New(), uuid.New(), "   ", "", nil, now); !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("blank title error = %v, want ErrInvalidTitle", err)
	}

	task := restoredTask(t, StatusOpen, now, now)
	updatedAt := now.Add(time.Minute)
	if err := task.UpdateTitle("Updated", updatedAt); err != nil {
		t.Fatalf("UpdateTitle: %v", err)
	}
	if task.Title != "Updated" || !task.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("task title/update mismatch: %#v", task)
	}
	if err := task.UpdateTitle("\t", updatedAt.Add(time.Minute)); !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("blank update title error = %v, want ErrInvalidTitle", err)
	}
}

func TestUpdateDescription(t *testing.T) {
	now := testTime()
	task := restoredTask(t, StatusOpen, now, now)
	updatedAt := now.Add(time.Minute)

	if err := task.UpdateDescription("", updatedAt); err != nil {
		t.Fatalf("UpdateDescription: %v", err)
	}

	if task.Description != "" || !task.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("task description/update mismatch: %#v", task)
	}
}

func TestUpdateDeadlineAllowsNilPastAndFuture(t *testing.T) {
	now := testTime()
	task := restoredTask(t, StatusOpen, now, now)
	pastDeadline := now.Add(-time.Hour)
	updatedAt := now.Add(time.Minute)

	if err := task.UpdateDeadline(&pastDeadline, updatedAt); err != nil {
		t.Fatalf("UpdateDeadline past: %v", err)
	}
	if task.DeadlineAt == nil || !task.DeadlineAt.Equal(pastDeadline) {
		t.Fatalf("DeadlineAt = %v, want %s", task.DeadlineAt, pastDeadline)
	}

	updatedAt = updatedAt.Add(time.Minute)
	if err := task.UpdateDeadline(nil, updatedAt); err != nil {
		t.Fatalf("UpdateDeadline nil: %v", err)
	}
	if task.DeadlineAt != nil {
		t.Fatalf("DeadlineAt = %v, want nil", task.DeadlineAt)
	}

	futureDeadline := now.Add(time.Hour)
	updatedAt = updatedAt.Add(time.Minute)
	if err := task.UpdateDeadline(&futureDeadline, updatedAt); err != nil {
		t.Fatalf("UpdateDeadline future: %v", err)
	}
	if task.DeadlineAt == nil || !task.DeadlineAt.Equal(futureDeadline) {
		t.Fatalf("DeadlineAt = %v, want %s", task.DeadlineAt, futureDeadline)
	}
}

func TestArchiveAndRestoreAreIdempotent(t *testing.T) {
	now := testTime()
	task := restoredTask(t, StatusOpen, now, now)
	archivedAt := now.Add(time.Minute)

	if err := task.Archive(archivedAt); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if !task.IsArchived() || task.ArchivedAt == nil || !task.ArchivedAt.Equal(archivedAt) {
		t.Fatalf("task should be archived: %#v", task)
	}

	snapshot := task
	if err := task.Archive(archivedAt.Add(time.Minute)); err != nil {
		t.Fatalf("Archive repeated: %v", err)
	}
	if !reflect.DeepEqual(task, snapshot) {
		t.Fatalf("repeated Archive should be no-op: got %#v want %#v", task, snapshot)
	}

	restoredAt := archivedAt.Add(time.Minute)
	if err := task.Restore(restoredAt); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if task.IsArchived() {
		t.Fatalf("task should be restored: %#v", task)
	}
	if !task.UpdatedAt.Equal(restoredAt) {
		t.Fatalf("UpdatedAt = %s, want %s", task.UpdatedAt, restoredAt)
	}

	snapshot = task
	if err := task.Restore(restoredAt.Add(time.Minute)); err != nil {
		t.Fatalf("Restore repeated: %v", err)
	}
	if !reflect.DeepEqual(task, snapshot) {
		t.Fatalf("repeated Restore should be no-op: got %#v want %#v", task, snapshot)
	}
}

func TestIsOverdue(t *testing.T) {
	now := testTime()
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	pastOpen := restoredTask(t, StatusOpen, now.Add(-time.Hour), now.Add(-time.Hour))
	pastOpen.DeadlineAt = &past
	if !pastOpen.IsOverdue(now) {
		t.Fatalf("past incomplete task should be overdue")
	}

	futureOpen := restoredTask(t, StatusOpen, now, now)
	futureOpen.DeadlineAt = &future
	if futureOpen.IsOverdue(now) {
		t.Fatalf("future task should not be overdue")
	}

	done := restoredTask(t, StatusDone, now.Add(-time.Hour), now.Add(-time.Hour))
	done.DeadlineAt = &past
	if done.IsOverdue(now) {
		t.Fatalf("completed task should not be overdue")
	}

	noDeadline := restoredTask(t, StatusOpen, now, now)
	if noDeadline.IsOverdue(now) {
		t.Fatalf("task without deadline should not be overdue")
	}
}

func TestReassignUpdatesAssigneeAndOpenTaskStatus(t *testing.T) {
	now := testTime()
	task := restoredTask(t, StatusOpen, now, now)
	newAssigneeID := uuid.MustParse("10000000-0000-4000-8000-000000000003")
	reassignedAt := now.Add(time.Minute)

	if err := task.Reassign(newAssigneeID, reassignedAt); err != nil {
		t.Fatalf("Reassign: %v", err)
	}

	if task.AssigneeID != newAssigneeID {
		t.Fatalf("AssigneeID = %s, want %s", task.AssigneeID, newAssigneeID)
	}
	if task.Status != StatusInProgress {
		t.Fatalf("Status = %s, want IN_PROGRESS", task.Status)
	}
	if !task.UpdatedAt.Equal(reassignedAt) {
		t.Fatalf("UpdatedAt = %s, want %s", task.UpdatedAt, reassignedAt)
	}
}

func TestReassignKeepsInProgressStatus(t *testing.T) {
	now := testTime()
	task := restoredTask(t, StatusInProgress, now, now)
	newAssigneeID := uuid.MustParse("10000000-0000-4000-8000-000000000003")

	if err := task.Reassign(newAssigneeID, now.Add(time.Minute)); err != nil {
		t.Fatalf("Reassign: %v", err)
	}

	if task.Status != StatusInProgress {
		t.Fatalf("Status = %s, want IN_PROGRESS", task.Status)
	}
}

func TestReassignRejectsTerminalStatuses(t *testing.T) {
	for _, status := range []Status{StatusDone, StatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			now := testTime()
			task := restoredTask(t, status, now, now)

			err := task.Reassign(uuid.New(), now.Add(time.Minute))
			if !errors.Is(err, ErrInvalidStatusTransition) {
				t.Fatalf("Reassign error = %v, want ErrInvalidStatusTransition", err)
			}
		})
	}
}

func TestMutationRejectsTimestampMovingBackwards(t *testing.T) {
	now := testTime()
	task := restoredTask(t, StatusOpen, now, now.Add(time.Minute))

	err := task.UpdateDescription("updated", now.Add(30*time.Second))
	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Fatalf("UpdateDescription error = %v, want ErrInvalidTimestamp", err)
	}
}

func testTime() time.Time {
	return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
}

func restoredTask(t *testing.T, status Status, createdAt time.Time, updatedAt time.Time) Task {
	t.Helper()

	var completedAt *time.Time
	if status.IsCompleted() {
		completedAt = &updatedAt
	}

	task, err := RestoreTask(
		uuid.MustParse("10000000-0000-4000-8000-000000000010"),
		nil,
		uuid.MustParse("10000000-0000-4000-8000-000000000001"),
		uuid.MustParse("10000000-0000-4000-8000-000000000002"),
		"Existing task",
		"description",
		status,
		nil,
		createdAt,
		updatedAt,
		completedAt,
		nil,
	)
	if err != nil {
		t.Fatalf("RestoreTask: %v", err)
	}

	return task
}
