package integration

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"go_pet_project/internal/platform/database"
	reminderscheduler "go_pet_project/internal/reminder/adapter/in/scheduler"
	reminderpostgres "go_pet_project/internal/reminder/adapter/out/postgres"
	reminderout "go_pet_project/internal/reminder/application/port/out"
	reminderdomain "go_pet_project/internal/reminder/domain"
	taskpostgres "go_pet_project/internal/task/adapter/out/postgres"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"

	"github.com/google/uuid"
)

func TestPostgresReminderClaimDueFiltersLimitAndOrdering(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_reminder_scheduler_claim_test_")
	_, reminders, taskID := setupReminderSchedulerRepositories(ctx, t, pg)
	now := schedulerIntegrationTime()

	dueOld := mustSchedulerReminder(t, taskID, now.Add(-3*time.Hour), now.Add(-24*time.Hour), reminderdomain.StatePending)
	dueNew := mustSchedulerReminder(t, taskID, now.Add(-time.Hour), now.Add(-24*time.Hour), reminderdomain.StatePending)
	future := mustSchedulerReminder(t, taskID, now.Add(time.Hour), now.Add(-24*time.Hour), reminderdomain.StatePending)
	sent := mustSchedulerReminder(t, taskID, now.Add(-2*time.Hour), now.Add(-24*time.Hour), reminderdomain.StateSent)
	cancelled := mustSchedulerReminder(t, taskID, now.Add(-90*time.Minute), now.Add(-24*time.Hour), reminderdomain.StateCancelled)
	for _, reminder := range []reminderdomain.Reminder{dueNew, dueOld, future, sent, cancelled} {
		if _, err := reminders.Create(ctx, reminder); err != nil {
			t.Fatalf("create reminder %s: %v", reminder.ID, err)
		}
	}

	var claimed []reminderdomain.Reminder
	count, err := reminders.ClaimDue(ctx, now, 1, func(_ context.Context, reminders []reminderdomain.Reminder) error {
		claimed = append(claimed, reminders...)
		return nil
	})
	if err != nil {
		t.Fatalf("ClaimDue limit 1: %v", err)
	}
	if count != 1 || len(claimed) != 1 || claimed[0].ID != dueOld.ID {
		t.Fatalf("claimed = %v count=%d, want oldest due only", reminderIDs(claimed), count)
	}

	claimed = nil
	count, err = reminders.ClaimDue(ctx, now, 10, func(_ context.Context, reminders []reminderdomain.Reminder) error {
		claimed = append(claimed, reminders...)
		return nil
	})
	if err != nil {
		t.Fatalf("ClaimDue limit 10: %v", err)
	}
	if count != 2 || len(claimed) != 2 || claimed[0].ID != dueOld.ID || claimed[1].ID != dueNew.ID {
		t.Fatalf("claimed = %v count=%d, want due reminders ordered oldest first", reminderIDs(claimed), count)
	}
	assertReminderStillPending(ctx, t, reminders, dueOld.ID)
	assertReminderStillPending(ctx, t, reminders, dueNew.ID)
}

func TestPostgresReminderClaimDueSkipLocked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_reminder_scheduler_skip_locked_test_")
	_, reminders, taskID := setupReminderSchedulerRepositories(ctx, t, pg)
	now := schedulerIntegrationTime()

	for i := 0; i < 3; i++ {
		reminder := mustSchedulerReminder(t, taskID, now.Add(time.Duration(-3+i)*time.Hour), now.Add(-24*time.Hour), reminderdomain.StatePending)
		if _, err := reminders.Create(ctx, reminder); err != nil {
			t.Fatalf("create reminder %d: %v", i, err)
		}
	}

	aClaimed := make(chan []uuid.UUID, 1)
	releaseA := make(chan struct{})
	aErr := make(chan error, 1)
	go func() {
		_, err := reminders.ClaimDue(ctx, now, 2, func(_ context.Context, reminders []reminderdomain.Reminder) error {
			aClaimed <- reminderIDs(reminders)
			<-releaseA
			return nil
		})
		aErr <- err
	}()

	var idsA []uuid.UUID
	select {
	case idsA = <-aClaimed:
	case <-time.After(5 * time.Second):
		t.Fatal("transaction A did not claim reminders")
	}
	if len(idsA) != 2 {
		t.Fatalf("transaction A claimed %d reminders, want 2: %v", len(idsA), idsA)
	}

	var idsB []uuid.UUID
	bDone := make(chan error, 1)
	go func() {
		_, err := reminders.ClaimDue(ctx, now, 3, func(_ context.Context, reminders []reminderdomain.Reminder) error {
			idsB = reminderIDs(reminders)
			return nil
		})
		bDone <- err
	}()

	select {
	case err := <-bDone:
		if err != nil {
			t.Fatalf("transaction B ClaimDue: %v", err)
		}
	case <-time.After(5 * time.Second):
		close(releaseA)
		t.Fatal("transaction B blocked instead of skipping locked rows")
	}
	close(releaseA)
	if err := <-aErr; err != nil {
		t.Fatalf("transaction A ClaimDue: %v", err)
	}

	if len(idsB) != 1 {
		t.Fatalf("transaction B claimed %d reminders, want 1: %v", len(idsB), idsB)
	}
	assertNoUUIDOverlap(t, idsA, idsB)
}

func TestPostgresReminderClaimDueConcurrentBatches(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_reminder_scheduler_batch_test_")
	_, reminders, taskID := setupReminderSchedulerRepositories(ctx, t, pg)
	now := schedulerIntegrationTime()

	for i := 0; i < 5; i++ {
		reminder := mustSchedulerReminder(t, taskID, now.Add(time.Duration(-10+i)*time.Minute), now.Add(-24*time.Hour), reminderdomain.StatePending)
		if _, err := reminders.Create(ctx, reminder); err != nil {
			t.Fatalf("create reminder %d: %v", i, err)
		}
	}

	aClaimed := make(chan []uuid.UUID, 1)
	releaseA := make(chan struct{})
	aErr := make(chan error, 1)
	go func() {
		_, err := reminders.ClaimDue(ctx, now, 2, func(_ context.Context, reminders []reminderdomain.Reminder) error {
			aClaimed <- reminderIDs(reminders)
			<-releaseA
			return nil
		})
		aErr <- err
	}()

	idsA := <-aClaimed
	var idsB []uuid.UUID
	countB, err := reminders.ClaimDue(ctx, now, 10, func(_ context.Context, reminders []reminderdomain.Reminder) error {
		idsB = reminderIDs(reminders)
		return nil
	})
	close(releaseA)
	if waitErr := <-aErr; waitErr != nil {
		t.Fatalf("transaction A ClaimDue: %v", waitErr)
	}
	if err != nil {
		t.Fatalf("transaction B ClaimDue: %v", err)
	}
	if len(idsA) != 2 {
		t.Fatalf("first batch claimed %d reminders, want 2", len(idsA))
	}
	if countB != 3 || len(idsB) != 3 {
		t.Fatalf("second batch claimed %d reminders count=%d, want 3", len(idsB), countB)
	}
	assertNoUUIDOverlap(t, idsA, idsB)
}

func TestReminderSchedulerTwoInstancesDoNotProcessSameReminderConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_reminder_scheduler_instances_test_")
	_, reminders, taskID := setupReminderSchedulerRepositories(ctx, t, pg)
	triggerAt := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	reminder := mustSchedulerReminder(t, taskID, triggerAt, triggerAt.Add(-time.Hour), reminderdomain.StatePending)
	if _, err := reminders.Create(ctx, reminder); err != nil {
		t.Fatalf("create reminder: %v", err)
	}

	startedA := make(chan struct{})
	releaseA := make(chan struct{})
	processorA := reminderscheduler.ProcessorFunc(func(_ context.Context, _ reminderdomain.Reminder) error {
		close(startedA)
		<-releaseA
		return nil
	})
	processedByB := make(chan uuid.UUID, 1)
	processorB := reminderscheduler.ProcessorFunc(func(_ context.Context, reminder reminderdomain.Reminder) error {
		processedByB <- reminder.ID
		return nil
	})

	runnerA := newIntegrationSchedulerRunner(t, reminders, processorA, 1, 1)
	runnerB := newIntegrationSchedulerRunner(t, reminders, processorB, 1, 1)

	errA := make(chan error, 1)
	go func() {
		_, err := runnerA.ProcessOnce(ctx)
		errA <- err
	}()

	select {
	case <-startedA:
	case <-time.After(5 * time.Second):
		t.Fatal("runner A did not start processing")
	}

	countB, err := runnerB.ProcessOnce(ctx)
	if err != nil {
		close(releaseA)
		t.Fatalf("runner B ProcessOnce: %v", err)
	}
	if countB != 0 {
		close(releaseA)
		t.Fatalf("runner B processed %d reminders while A held row lock, want 0", countB)
	}
	select {
	case id := <-processedByB:
		close(releaseA)
		t.Fatalf("runner B processed locked reminder %s", id)
	default:
	}

	close(releaseA)
	if err := <-errA; err != nil {
		t.Fatalf("runner A ProcessOnce: %v", err)
	}
}

func setupReminderSchedulerRepositories(ctx context.Context, t *testing.T, pg *database.Postgres) (*taskpostgres.Repository, *reminderpostgres.Repository, uuid.UUID) {
	t.Helper()

	users := userpostgres.NewRepository(pg.GORM)
	tasks := taskpostgres.NewRepository(pg.GORM)
	reminders := reminderpostgres.NewRepository(pg.GORM)
	creator := createTaskUser(ctx, t, users, "scheduler-creator-"+uuid.NewString()+"@example.com", "scheduler_creator_"+uuid.NewString()[:8])
	assignee := createTaskUser(ctx, t, users, "scheduler-assignee-"+uuid.NewString()+"@example.com", "scheduler_assignee_"+uuid.NewString()[:8])
	task := createRepositoryTask(ctx, t, tasks, creator.ID, assignee.ID, "scheduler reminders", schedulerIntegrationTime().Add(-24*time.Hour))

	return tasks, reminders, task.ID
}

func mustSchedulerReminder(t *testing.T, taskID uuid.UUID, triggerAt time.Time, createdAt time.Time, state reminderdomain.State) reminderdomain.Reminder {
	t.Helper()

	var sentAt *time.Time
	if state == reminderdomain.StateSent {
		value := createdAt.Add(time.Minute)
		sentAt = &value
	}
	reminder, err := reminderdomain.RestoreReminder(uuid.New(), taskID, reminderdomain.KindAbsolute, nil, triggerAt, state, createdAt, sentAt)
	if err != nil {
		t.Fatalf("RestoreReminder: %v", err)
	}

	return reminder
}

func newIntegrationSchedulerRunner(
	t *testing.T,
	claimer reminderout.DueReminderClaimer,
	processor reminderscheduler.Processor,
	batchSize int,
	workers int,
) *reminderscheduler.Runner {
	t.Helper()

	runner, err := reminderscheduler.NewRunner(claimer, processor, reminderscheduler.Config{
		Interval:  time.Hour,
		BatchSize: batchSize,
		Workers:   workers,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	return runner
}

func reminderIDs(reminders []reminderdomain.Reminder) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(reminders))
	for _, reminder := range reminders {
		ids = append(ids, reminder.ID)
	}

	return ids
}

func assertNoUUIDOverlap(t *testing.T, left []uuid.UUID, right []uuid.UUID) {
	t.Helper()

	seen := make(map[uuid.UUID]struct{}, len(left))
	for _, id := range left {
		seen[id] = struct{}{}
	}
	for _, id := range right {
		if _, ok := seen[id]; ok {
			t.Fatalf("id %s was claimed by both batches: left=%v right=%v", id, left, right)
		}
	}
}

func assertReminderStillPending(ctx context.Context, t *testing.T, reminders *reminderpostgres.Repository, id uuid.UUID) {
	t.Helper()

	reminder, err := reminders.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID %s: %v", id, err)
	}
	if reminder.State != reminderdomain.StatePending {
		t.Fatalf("reminder %s State = %s, want PENDING", id, reminder.State)
	}
}

func schedulerIntegrationTime() time.Time {
	return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
}
