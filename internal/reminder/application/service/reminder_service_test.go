package service

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"go_pet_project/internal/reminder/application"
	"go_pet_project/internal/reminder/application/command"
	"go_pet_project/internal/reminder/application/query"
	"go_pet_project/internal/reminder/domain"
	taskdomain "go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

func TestReminderServiceCreatorCRUD(t *testing.T) {
	now := reminderTestTime()
	deadline := now.Add(24 * time.Hour)
	task := reminderTestTask(t, reminderTestUUID(1), reminderTestUUID(2), taskdomain.StatusOpen, &deadline)
	repo := newFakeReminderRepository()
	service := newTestReminderService(t, &fakeTaskReader{task: task}, repo, now)

	absoluteAt := now.Add(2 * time.Hour)
	createdAbsolute, err := service.CreateReminder(context.Background(), command.CreateReminderCommand{
		ActorID:   task.CreatorID,
		TaskID:    task.ID,
		Kind:      domain.KindAbsolute,
		TriggerAt: &absoluteAt,
	})
	if err != nil {
		t.Fatalf("CreateReminder absolute: %v", err)
	}
	if createdAbsolute.Kind != domain.KindAbsolute || createdAbsolute.State != domain.StatePending {
		t.Fatalf("unexpected absolute reminder: %#v", createdAbsolute)
	}

	offset := int64(7200)
	createdRelative, err := service.CreateReminder(context.Background(), command.CreateReminderCommand{
		ActorID:       task.CreatorID,
		TaskID:        task.ID,
		Kind:          domain.KindBeforeDeadline,
		OffsetSeconds: &offset,
	})
	if err != nil {
		t.Fatalf("CreateReminder relative: %v", err)
	}
	if !createdRelative.TriggerAt.Equal(deadline.Add(-2 * time.Hour)) {
		t.Fatalf("relative TriggerAt = %s, want %s", createdRelative.TriggerAt, deadline.Add(-2*time.Hour))
	}

	listed, err := service.ListReminders(context.Background(), query.ListRemindersQuery{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
	})
	if err != nil {
		t.Fatalf("ListReminders: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d reminders, want 2", len(listed))
	}

	nextAbsoluteAt := now.Add(3 * time.Hour)
	updatedAbsolute, err := service.UpdateReminder(context.Background(), command.UpdateReminderCommand{
		ActorID:    task.CreatorID,
		TaskID:     task.ID,
		ReminderID: createdAbsolute.ID,
		TriggerAt:  &nextAbsoluteAt,
	})
	if err != nil {
		t.Fatalf("UpdateReminder absolute: %v", err)
	}
	if !updatedAbsolute.TriggerAt.Equal(nextAbsoluteAt) {
		t.Fatalf("absolute TriggerAt = %s, want %s", updatedAbsolute.TriggerAt, nextAbsoluteAt)
	}

	nextOffset := int64(3600)
	updatedRelative, err := service.UpdateReminder(context.Background(), command.UpdateReminderCommand{
		ActorID:       task.CreatorID,
		TaskID:        task.ID,
		ReminderID:    createdRelative.ID,
		OffsetSeconds: &nextOffset,
	})
	if err != nil {
		t.Fatalf("UpdateReminder relative: %v", err)
	}
	if updatedRelative.OffsetSeconds == nil ||
		*updatedRelative.OffsetSeconds != nextOffset ||
		!updatedRelative.TriggerAt.Equal(deadline.Add(-time.Hour)) {
		t.Fatalf("unexpected updated relative reminder: %#v", updatedRelative)
	}

	if err := service.DeleteReminder(context.Background(), command.DeleteReminderCommand{
		ActorID:    task.CreatorID,
		TaskID:     task.ID,
		ReminderID: createdAbsolute.ID,
	}); err != nil {
		t.Fatalf("DeleteReminder: %v", err)
	}
	if _, err := repo.FindByID(context.Background(), createdAbsolute.ID); !errors.Is(err, domain.ErrReminderNotFound) {
		t.Fatalf("deleted FindByID error = %v, want ErrReminderNotFound", err)
	}
}

func TestReminderServiceAccessRules(t *testing.T) {
	now := reminderTestTime()
	task := reminderTestTask(t, reminderTestUUID(1), reminderTestUUID(2), taskdomain.StatusOpen, nil)
	repo := newFakeReminderRepository()
	service := newTestReminderService(t, &fakeTaskReader{task: task}, repo, now)

	reminder := mustAbsoluteReminder(t, task.ID, now.Add(time.Hour), now)
	if _, err := repo.Create(context.Background(), reminder); err != nil {
		t.Fatalf("seed reminder: %v", err)
	}

	if _, err := service.ListReminders(context.Background(), query.ListRemindersQuery{ActorID: task.AssigneeID, TaskID: task.ID}); err != nil {
		t.Fatalf("assignee ListReminders: %v", err)
	}

	offset := int64(60)
	_, createErr := service.CreateReminder(context.Background(), command.CreateReminderCommand{
		ActorID:       task.AssigneeID,
		TaskID:        task.ID,
		Kind:          domain.KindBeforeDeadline,
		OffsetSeconds: &offset,
	})
	if !errors.Is(createErr, application.ErrReminderAccessDenied) {
		t.Fatalf("assignee create error = %v, want ErrReminderAccessDenied", createErr)
	}

	_, updateErr := service.UpdateReminder(context.Background(), command.UpdateReminderCommand{
		ActorID:    task.AssigneeID,
		TaskID:     task.ID,
		ReminderID: reminder.ID,
		TriggerAt:  timePtrForReminderTest(now.Add(2 * time.Hour)),
	})
	if !errors.Is(updateErr, application.ErrReminderAccessDenied) {
		t.Fatalf("assignee update error = %v, want ErrReminderAccessDenied", updateErr)
	}

	deleteErr := service.DeleteReminder(context.Background(), command.DeleteReminderCommand{
		ActorID:    task.AssigneeID,
		TaskID:     task.ID,
		ReminderID: reminder.ID,
	})
	if !errors.Is(deleteErr, application.ErrReminderAccessDenied) {
		t.Fatalf("assignee delete error = %v, want ErrReminderAccessDenied", deleteErr)
	}

	_, unrelatedErr := service.ListReminders(context.Background(), query.ListRemindersQuery{
		ActorID: reminderTestUUID(3),
		TaskID:  task.ID,
	})
	if !errors.Is(unrelatedErr, application.ErrReminderAccessDenied) {
		t.Fatalf("unrelated list error = %v, want ErrReminderAccessDenied", unrelatedErr)
	}
}

func TestReminderServiceRelativeWithoutDeadline(t *testing.T) {
	now := reminderTestTime()
	task := reminderTestTask(t, reminderTestUUID(1), reminderTestUUID(1), taskdomain.StatusOpen, nil)
	service := newTestReminderService(t, &fakeTaskReader{task: task}, newFakeReminderRepository(), now)
	offset := int64(7200)

	_, err := service.CreateReminder(context.Background(), command.CreateReminderCommand{
		ActorID:       task.CreatorID,
		TaskID:        task.ID,
		Kind:          domain.KindBeforeDeadline,
		OffsetSeconds: &offset,
	})
	if !errors.Is(err, domain.ErrTaskDeadlineRequired) {
		t.Fatalf("CreateReminder error = %v, want ErrTaskDeadlineRequired", err)
	}
}

func TestReminderServiceRejectsCompletedTask(t *testing.T) {
	now := reminderTestTime()
	task := reminderTestTask(t, reminderTestUUID(1), reminderTestUUID(1), taskdomain.StatusDone, nil)
	service := newTestReminderService(t, &fakeTaskReader{task: task}, newFakeReminderRepository(), now)
	triggerAt := now.Add(time.Hour)

	_, err := service.CreateReminder(context.Background(), command.CreateReminderCommand{
		ActorID:   task.CreatorID,
		TaskID:    task.ID,
		Kind:      domain.KindAbsolute,
		TriggerAt: &triggerAt,
	})
	if !errors.Is(err, domain.ErrTaskAlreadyCompleted) {
		t.Fatalf("CreateReminder error = %v, want ErrTaskAlreadyCompleted", err)
	}
}

func TestReminderServiceNestedTaskReminderMismatch(t *testing.T) {
	now := reminderTestTime()
	pathTask := reminderTestTask(t, reminderTestUUID(1), reminderTestUUID(1), taskdomain.StatusOpen, nil)
	otherTaskID := reminderTestUUID(2)
	repo := newFakeReminderRepository()
	service := newTestReminderService(t, &fakeTaskReader{task: pathTask}, repo, now)
	reminder := mustAbsoluteReminder(t, otherTaskID, now.Add(time.Hour), now)
	if _, err := repo.Create(context.Background(), reminder); err != nil {
		t.Fatalf("seed reminder: %v", err)
	}

	_, err := service.UpdateReminder(context.Background(), command.UpdateReminderCommand{
		ActorID:    pathTask.CreatorID,
		TaskID:     pathTask.ID,
		ReminderID: reminder.ID,
		TriggerAt:  timePtrForReminderTest(now.Add(2 * time.Hour)),
	})
	if !errors.Is(err, domain.ErrReminderNotFound) {
		t.Fatalf("UpdateReminder error = %v, want ErrReminderNotFound", err)
	}

	err = service.DeleteReminder(context.Background(), command.DeleteReminderCommand{
		ActorID:    pathTask.CreatorID,
		TaskID:     pathTask.ID,
		ReminderID: reminder.ID,
	})
	if !errors.Is(err, domain.ErrReminderNotFound) {
		t.Fatalf("DeleteReminder error = %v, want ErrReminderNotFound", err)
	}
}

func newTestReminderService(t *testing.T, tasks *fakeTaskReader, reminders *fakeReminderRepository, now time.Time) *ReminderService {
	t.Helper()

	service, err := NewReminderService(tasks, reminders)
	if err != nil {
		t.Fatalf("NewReminderService: %v", err)
	}
	service.now = func() time.Time { return now }

	return service
}

type fakeTaskReader struct {
	task taskdomain.Task
	err  error
}

func (r *fakeTaskReader) FindByID(_ context.Context, _ uuid.UUID) (taskdomain.Task, error) {
	if r.err != nil {
		return taskdomain.Task{}, r.err
	}

	return r.task, nil
}

type fakeReminderRepository struct {
	reminders map[uuid.UUID]domain.Reminder
}

func newFakeReminderRepository() *fakeReminderRepository {
	return &fakeReminderRepository{reminders: map[uuid.UUID]domain.Reminder{}}
}

func (r *fakeReminderRepository) Create(_ context.Context, reminder domain.Reminder) (domain.Reminder, error) {
	r.reminders[reminder.ID] = reminder
	return reminder, nil
}

func (r *fakeReminderRepository) FindByID(_ context.Context, id uuid.UUID) (domain.Reminder, error) {
	reminder, ok := r.reminders[id]
	if !ok {
		return domain.Reminder{}, domain.ErrReminderNotFound
	}

	return reminder, nil
}

func (r *fakeReminderRepository) ListByTaskID(_ context.Context, taskID uuid.UUID) ([]domain.Reminder, error) {
	var reminders []domain.Reminder
	for _, reminder := range r.reminders {
		if reminder.TaskID == taskID {
			reminders = append(reminders, reminder)
		}
	}
	sort.Slice(reminders, func(i, j int) bool {
		return reminders[i].TriggerAt.Before(reminders[j].TriggerAt)
	})

	return reminders, nil
}

func (r *fakeReminderRepository) Update(_ context.Context, reminder domain.Reminder) (domain.Reminder, error) {
	if _, ok := r.reminders[reminder.ID]; !ok {
		return domain.Reminder{}, domain.ErrReminderNotFound
	}
	r.reminders[reminder.ID] = reminder

	return reminder, nil
}

func (r *fakeReminderRepository) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := r.reminders[id]; !ok {
		return domain.ErrReminderNotFound
	}
	delete(r.reminders, id)

	return nil
}

func (r *fakeReminderRepository) HasPendingBeforeDeadline(_ context.Context, taskID uuid.UUID) (bool, error) {
	for _, reminder := range r.reminders {
		if reminder.TaskID == taskID && reminder.Kind == domain.KindBeforeDeadline && reminder.State == domain.StatePending {
			return true, nil
		}
	}

	return false, nil
}

func (r *fakeReminderRepository) RecalculatePendingBeforeDeadline(_ context.Context, taskID uuid.UUID, deadlineAt time.Time) error {
	for id, reminder := range r.reminders {
		if reminder.TaskID == taskID && reminder.Kind == domain.KindBeforeDeadline && reminder.State == domain.StatePending {
			if err := reminder.RecalculateForDeadline(deadlineAt); err != nil {
				return err
			}
			r.reminders[id] = reminder
		}
	}

	return nil
}

func (r *fakeReminderRepository) CancelPendingByTaskID(_ context.Context, taskID uuid.UUID) error {
	for id, reminder := range r.reminders {
		if reminder.TaskID == taskID && reminder.State == domain.StatePending {
			if err := reminder.Cancel(); err != nil {
				return err
			}
			r.reminders[id] = reminder
		}
	}

	return nil
}

func reminderTestTask(t *testing.T, creatorID uuid.UUID, assigneeID uuid.UUID, status taskdomain.Status, deadlineAt *time.Time) taskdomain.Task {
	t.Helper()

	now := reminderTestTime()
	completedAt := (*time.Time)(nil)
	if status.IsCompleted() {
		completedAt = timePtrForReminderTest(now)
	}
	task, err := taskdomain.RestoreTask(reminderTestUUID(100), nil, creatorID, assigneeID, "Reminder task", "", status, deadlineAt, now, now, completedAt, nil)
	if err != nil {
		t.Fatalf("RestoreTask: %v", err)
	}

	return task
}

func mustAbsoluteReminder(t *testing.T, taskID uuid.UUID, triggerAt time.Time, createdAt time.Time) domain.Reminder {
	t.Helper()

	reminder, err := domain.NewAbsoluteReminder(uuid.New(), taskID, triggerAt, createdAt)
	if err != nil {
		t.Fatalf("NewAbsoluteReminder: %v", err)
	}

	return reminder
}

func reminderTestTime() time.Time {
	return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
}

func reminderTestUUID(value byte) uuid.UUID {
	return uuid.UUID{15: value}
}

func timePtrForReminderTest(value time.Time) *time.Time {
	return &value
}
