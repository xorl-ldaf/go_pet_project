package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"go_pet_project/internal/platform/database"
	reminderpostgres "go_pet_project/internal/reminder/adapter/out/postgres"
	reminderservice "go_pet_project/internal/reminder/application/service"
	reminderdomain "go_pet_project/internal/reminder/domain"
	taskpostgres "go_pet_project/internal/task/adapter/out/postgres"
	taskcmd "go_pet_project/internal/task/application/command"
	taskout "go_pet_project/internal/task/application/port/out"
	taskservice "go_pet_project/internal/task/application/service"
	taskdomain "go_pet_project/internal/task/domain"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"

	"github.com/google/uuid"
)

func TestPostgresReminderRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_reminder_repository_test_")
	users := userpostgres.NewRepository(pg.GORM)
	tasks := taskpostgres.NewRepository(pg.GORM)
	reminders := reminderpostgres.NewRepository(pg.GORM)

	creator := createTaskUser(ctx, t, users, "reminder-creator@example.com", "reminder_creator")
	assignee := createTaskUser(ctx, t, users, "reminder-assignee@example.com", "reminder_assignee")
	task := createRepositoryTaskWithDeadline(ctx, t, tasks, creator.ID, assignee.ID, "reminder task", "", testTaskTime(1), testTaskTime(120))

	t.Run("create find list update delete", func(t *testing.T) {
		createdAt := testTaskTime(2)
		later := mustReminderAbsolute(t, task.ID, testTaskTime(20), createdAt)
		earlier := mustReminderRelative(t, task.ID, testTaskTime(10), createdAt, 3600)

		later, err := reminders.Create(ctx, later)
		if err != nil {
			t.Fatalf("create later reminder: %v", err)
		}
		earlier, err = reminders.Create(ctx, earlier)
		if err != nil {
			t.Fatalf("create earlier reminder: %v", err)
		}

		found, err := reminders.FindByID(ctx, later.ID)
		if err != nil {
			t.Fatalf("find reminder: %v", err)
		}
		if found.ID != later.ID || found.Kind != reminderdomain.KindAbsolute {
			t.Fatalf("found reminder mismatch: %#v", found)
		}

		listed, err := reminders.ListByTaskID(ctx, task.ID)
		if err != nil {
			t.Fatalf("list reminders: %v", err)
		}
		if len(listed) < 2 || listed[0].ID != earlier.ID || listed[1].ID != later.ID {
			t.Fatalf("list order mismatch: %#v", listed)
		}

		nextTrigger := testTaskTime(30)
		if err := later.RescheduleAbsolute(nextTrigger); err != nil {
			t.Fatalf("reschedule absolute: %v", err)
		}
		updated, err := reminders.Update(ctx, later)
		if err != nil {
			t.Fatalf("update reminder: %v", err)
		}
		if !updated.TriggerAt.Equal(nextTrigger) {
			t.Fatalf("updated TriggerAt = %s, want %s", updated.TriggerAt, nextTrigger)
		}

		if err := reminders.Delete(ctx, earlier.ID); err != nil {
			t.Fatalf("delete reminder: %v", err)
		}
		if _, err := reminders.FindByID(ctx, earlier.ID); !errors.Is(err, reminderdomain.ErrReminderNotFound) {
			t.Fatalf("find deleted error = %v, want ErrReminderNotFound", err)
		}
	})

	t.Run("bulk relative recalculation and pending cancellation", func(t *testing.T) {
		createdAt := testTaskTime(40)
		deadline := testTaskTime(240)
		pendingRelative := mustReminderRelative(t, task.ID, deadline.Add(-time.Hour), createdAt, 3600)
		pendingAbsolute := mustReminderAbsolute(t, task.ID, testTaskTime(80), createdAt)
		sentRelative := mustReminderRelative(t, task.ID, deadline.Add(-2*time.Hour), createdAt, 7200)
		sentAt := createdAt.Add(time.Minute)
		if err := sentRelative.MarkSent(sentAt); err != nil {
			t.Fatalf("mark sent: %v", err)
		}
		cancelledRelative := mustReminderRelative(t, task.ID, deadline.Add(-3*time.Hour), createdAt, 10800)
		if err := cancelledRelative.Cancel(); err != nil {
			t.Fatalf("cancel reminder: %v", err)
		}

		for _, reminder := range []reminderdomain.Reminder{pendingRelative, pendingAbsolute, sentRelative, cancelledRelative} {
			if _, err := reminders.Create(ctx, reminder); err != nil {
				t.Fatalf("seed reminder %s: %v", reminder.ID, err)
			}
		}

		nextDeadline := deadline.Add(24 * time.Hour)
		if err := reminders.RecalculatePendingBeforeDeadline(ctx, task.ID, nextDeadline); err != nil {
			t.Fatalf("recalculate pending relative: %v", err)
		}

		foundPendingRelative := findReminderByID(ctx, t, reminders, pendingRelative.ID)
		foundPendingAbsolute := findReminderByID(ctx, t, reminders, pendingAbsolute.ID)
		foundSentRelative := findReminderByID(ctx, t, reminders, sentRelative.ID)
		foundCancelledRelative := findReminderByID(ctx, t, reminders, cancelledRelative.ID)

		if !foundPendingRelative.TriggerAt.Equal(nextDeadline.Add(-time.Hour)) {
			t.Fatalf("pending relative TriggerAt = %s, want %s", foundPendingRelative.TriggerAt, nextDeadline.Add(-time.Hour))
		}
		if !foundPendingAbsolute.TriggerAt.Equal(pendingAbsolute.TriggerAt) {
			t.Fatalf("absolute reminder changed")
		}
		if !foundSentRelative.TriggerAt.Equal(sentRelative.TriggerAt) {
			t.Fatalf("sent relative changed")
		}
		if !foundCancelledRelative.TriggerAt.Equal(cancelledRelative.TriggerAt) {
			t.Fatalf("cancelled relative changed")
		}

		if err := reminders.CancelPendingByTaskID(ctx, task.ID); err != nil {
			t.Fatalf("cancel pending reminders: %v", err)
		}
		if findReminderByID(ctx, t, reminders, pendingRelative.ID).State != reminderdomain.StateCancelled {
			t.Fatalf("pending relative was not cancelled")
		}
		if findReminderByID(ctx, t, reminders, pendingAbsolute.ID).State != reminderdomain.StateCancelled {
			t.Fatalf("pending absolute was not cancelled")
		}
		if findReminderByID(ctx, t, reminders, sentRelative.ID).State != reminderdomain.StateSent {
			t.Fatalf("sent reminder state changed")
		}
		if findReminderByID(ctx, t, reminders, cancelledRelative.ID).State != reminderdomain.StateCancelled {
			t.Fatalf("cancelled reminder state changed")
		}
	})
}

func TestTaskReminderIntegrationAndRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_reminder_task_integration_test_")
	users := userpostgres.NewRepository(pg.GORM)
	taskRepo := taskpostgres.NewRepository(pg.GORM)
	reminderRepo := reminderpostgres.NewRepository(pg.GORM)
	transactionRunner := database.NewTransactionRunner(pg.GORM)

	creator := createTaskUser(ctx, t, users, "reminder-int-creator@example.com", "reminder_int_creator")
	assignee := createTaskUser(ctx, t, users, "reminder-int-assignee@example.com", "reminder_int_assignee")

	t.Run("deadline change recalculates relative only", func(t *testing.T) {
		task := createRepositoryTaskWithDeadline(ctx, t, taskRepo, creator.ID, assignee.ID, "deadline integration", "", testTaskTime(1), testTaskTime(120))
		relative := mustReminderRelative(t, task.ID, task.DeadlineAt.Add(-time.Hour), testTaskTime(2), 3600)
		absolute := mustReminderAbsolute(t, task.ID, testTaskTime(60), testTaskTime(2))
		if _, err := reminderRepo.Create(ctx, relative); err != nil {
			t.Fatalf("create relative reminder: %v", err)
		}
		if _, err := reminderRepo.Create(ctx, absolute); err != nil {
			t.Fatalf("create absolute reminder: %v", err)
		}

		reminderService := newIntegrationReminderService(t, taskRepo, reminderRepo)
		taskService := newIntegrationTaskService(t, taskRepo, reminderService, transactionRunner)
		nextDeadline := testTaskTime(240)
		_, err := taskService.UpdateTask(ctx, taskcmd.UpdateTaskCommand{
			ActorID:    task.CreatorID,
			TaskID:     task.ID,
			DeadlineAt: &taskcmd.DeadlineUpdate{Value: &nextDeadline},
		})
		if err != nil {
			t.Fatalf("UpdateTask deadline: %v", err)
		}

		if got := findReminderByID(ctx, t, reminderRepo, relative.ID); !got.TriggerAt.Equal(nextDeadline.Add(-time.Hour)) {
			t.Fatalf("relative TriggerAt = %s, want %s", got.TriggerAt, nextDeadline.Add(-time.Hour))
		}
		if got := findReminderByID(ctx, t, reminderRepo, absolute.ID); !got.TriggerAt.Equal(absolute.TriggerAt) {
			t.Fatalf("absolute TriggerAt changed")
		}
	})

	t.Run("completion cancels pending reminders", func(t *testing.T) {
		task := createRepositoryTaskWithDeadline(ctx, t, taskRepo, creator.ID, assignee.ID, "completion integration", "", testTaskTime(10), testTaskTime(180))
		pending := mustReminderAbsolute(t, task.ID, testTaskTime(90), testTaskTime(11))
		sent := mustReminderAbsolute(t, task.ID, testTaskTime(100), testTaskTime(11))
		if err := sent.MarkSent(testTaskTime(12)); err != nil {
			t.Fatalf("mark sent: %v", err)
		}
		if _, err := reminderRepo.Create(ctx, pending); err != nil {
			t.Fatalf("create pending reminder: %v", err)
		}
		if _, err := reminderRepo.Create(ctx, sent); err != nil {
			t.Fatalf("create sent reminder: %v", err)
		}

		reminderService := newIntegrationReminderService(t, taskRepo, reminderRepo)
		taskService := newIntegrationTaskService(t, taskRepo, reminderService, transactionRunner)
		_, err := taskService.ChangeStatus(ctx, taskcmd.ChangeStatusCommand{
			ActorID: task.AssigneeID,
			TaskID:  task.ID,
			Status:  taskdomain.StatusDone,
		})
		if err != nil {
			t.Fatalf("ChangeStatus done: %v", err)
		}

		if got := findReminderByID(ctx, t, reminderRepo, pending.ID); got.State != reminderdomain.StateCancelled {
			t.Fatalf("pending reminder State = %s, want CANCELLED", got.State)
		}
		if got := findReminderByID(ctx, t, reminderRepo, sent.ID); got.State != reminderdomain.StateSent {
			t.Fatalf("sent reminder State = %s, want SENT", got.State)
		}
	})

	t.Run("deadline recalculation rolls back when task update fails", func(t *testing.T) {
		task := createRepositoryTaskWithDeadline(ctx, t, taskRepo, creator.ID, assignee.ID, "deadline rollback", "", testTaskTime(20), testTaskTime(200))
		relative := mustReminderRelative(t, task.ID, task.DeadlineAt.Add(-time.Hour), testTaskTime(21), 3600)
		if _, err := reminderRepo.Create(ctx, relative); err != nil {
			t.Fatalf("create relative reminder: %v", err)
		}

		reminderService := newIntegrationReminderService(t, taskRepo, reminderRepo)
		failingTasks := &failingTaskRepository{TaskRepository: taskRepo, updateErr: errRollbackTaskUpdate}
		taskService := newIntegrationTaskService(t, failingTasks, reminderService, transactionRunner)
		nextDeadline := testTaskTime(300)
		_, err := taskService.UpdateTask(ctx, taskcmd.UpdateTaskCommand{
			ActorID:    task.CreatorID,
			TaskID:     task.ID,
			DeadlineAt: &taskcmd.DeadlineUpdate{Value: &nextDeadline},
		})
		if !errors.Is(err, errRollbackTaskUpdate) {
			t.Fatalf("UpdateTask error = %v, want rollback task update error", err)
		}
		if got := findReminderByID(ctx, t, reminderRepo, relative.ID); !got.TriggerAt.Equal(relative.TriggerAt) {
			t.Fatalf("relative reminder was not rolled back: got %s want %s", got.TriggerAt, relative.TriggerAt)
		}
	})

	t.Run("completion cancellation rolls back when task update fails", func(t *testing.T) {
		task := createRepositoryTaskWithDeadline(ctx, t, taskRepo, creator.ID, assignee.ID, "completion rollback", "", testTaskTime(30), testTaskTime(210))
		pending := mustReminderAbsolute(t, task.ID, testTaskTime(120), testTaskTime(31))
		if _, err := reminderRepo.Create(ctx, pending); err != nil {
			t.Fatalf("create pending reminder: %v", err)
		}

		reminderService := newIntegrationReminderService(t, taskRepo, reminderRepo)
		failingTasks := &failingTaskRepository{TaskRepository: taskRepo, updateErr: errRollbackTaskUpdate}
		taskService := newIntegrationTaskService(t, failingTasks, reminderService, transactionRunner)
		_, err := taskService.ChangeStatus(ctx, taskcmd.ChangeStatusCommand{
			ActorID: task.AssigneeID,
			TaskID:  task.ID,
			Status:  taskdomain.StatusDone,
		})
		if !errors.Is(err, errRollbackTaskUpdate) {
			t.Fatalf("ChangeStatus error = %v, want rollback task update error", err)
		}
		if got := findReminderByID(ctx, t, reminderRepo, pending.ID); got.State != reminderdomain.StatePending {
			t.Fatalf("pending reminder was not rolled back: state %s", got.State)
		}
	})
}

var errRollbackTaskUpdate = errors.New("forced task update failure")

type failingTaskRepository struct {
	taskout.TaskRepository
	updateErr error
}

func (r *failingTaskRepository) Update(_ context.Context, _ taskdomain.Task) (taskdomain.Task, error) {
	return taskdomain.Task{}, r.updateErr
}

type allowAllAssignmentAuthorizer struct{}

func (a allowAllAssignmentAuthorizer) CanAssign(_ context.Context, _ uuid.UUID, _ uuid.UUID) (bool, error) {
	return true, nil
}

func newIntegrationReminderService(t *testing.T, tasks *taskpostgres.Repository, reminders *reminderpostgres.Repository) *reminderservice.ReminderService {
	t.Helper()

	service, err := reminderservice.NewReminderService(tasks, reminders)
	if err != nil {
		t.Fatalf("NewReminderService: %v", err)
	}

	return service
}

func newIntegrationTaskService(
	t *testing.T,
	tasks taskout.TaskRepository,
	reminders taskout.ReminderManager,
	transactions taskout.TransactionRunner,
) *taskservice.TaskService {
	t.Helper()

	service, err := taskservice.NewTaskService(tasks, allowAllAssignmentAuthorizer{}, reminders, transactions)
	if err != nil {
		t.Fatalf("NewTaskService: %v", err)
	}

	return service
}

func mustReminderAbsolute(t *testing.T, taskID uuid.UUID, triggerAt time.Time, createdAt time.Time) reminderdomain.Reminder {
	t.Helper()

	reminder, err := reminderdomain.NewAbsoluteReminder(uuid.New(), taskID, triggerAt, createdAt)
	if err != nil {
		t.Fatalf("NewAbsoluteReminder: %v", err)
	}

	return reminder
}

func mustReminderRelative(t *testing.T, taskID uuid.UUID, triggerAt time.Time, createdAt time.Time, offsetSeconds int64) reminderdomain.Reminder {
	t.Helper()

	reminder, err := reminderdomain.RestoreReminder(
		uuid.New(),
		taskID,
		reminderdomain.KindBeforeDeadline,
		&offsetSeconds,
		triggerAt,
		reminderdomain.StatePending,
		createdAt,
		nil,
	)
	if err != nil {
		t.Fatalf("RestoreReminder relative: %v", err)
	}

	return reminder
}

func findReminderByID(ctx context.Context, t *testing.T, reminders *reminderpostgres.Repository, id uuid.UUID) reminderdomain.Reminder {
	t.Helper()

	reminder, err := reminders.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID %s: %v", id, err)
	}

	return reminder
}

var _ taskout.AssignmentAuthorizer = allowAllAssignmentAuthorizer{}
