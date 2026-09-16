package integration

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/migrations"
	taskpostgres "go_pet_project/internal/task/adapter/out/postgres"
	taskout "go_pet_project/internal/task/application/port/out"
	taskdomain "go_pet_project/internal/task/domain"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"
	userdomain "go_pet_project/internal/user/domain"

	"github.com/google/uuid"
)

func TestPostgresTaskRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	startPostgres(ctx, t)
	waitForPostgres(ctx, t, cfg.DB)

	testDBName := "todo_task_repository_test_" + uuid.NewString()
	adminDB := openAdminDB(ctx, t, cfg.DB)
	createTestDatabase(ctx, t, adminDB, testDBName)
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer dropCancel()
		dropTestDatabase(dropCtx, t, adminDB, testDBName)
		adminDB.Close()
	})

	testCfg := cfg.DB
	testCfg.Name = testDBName

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrations.Up(ctx, testCfg, migrationsDir, logger); err != nil {
		t.Fatalf("migration up: %v", err)
	}

	pg, err := database.Open(ctx, testCfg, logger)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pg.Close()

	users := userpostgres.NewRepository(pg.GORM)
	tasks := taskpostgres.NewRepository(pg.GORM)
	creator := createTaskUser(ctx, t, users, "task-creator@example.com", "task_creator")
	assignee := createTaskUser(ctx, t, users, "task-assignee@example.com", "task_assignee")
	otherAssignee := createTaskUser(ctx, t, users, "task-other-assignee@example.com", "task_other_assignee")
	otherCreator := createTaskUser(ctx, t, users, "task-other-creator@example.com", "task_other_creator")

	t.Run("create and find by id round trip", func(t *testing.T) {
		task := newRepositoryTask(t, creator.ID, assignee.ID, "round trip", testTaskTime(1))
		seriesID := createTaskSeriesFixture(ctx, t, pg.SQL, creator.ID, assignee.ID)
		task.SeriesID = &seriesID
		deadline := task.CreatedAt.Add(24 * time.Hour)
		task.DeadlineAt = &deadline

		created, err := tasks.Create(ctx, task)
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		assertTaskEqual(t, created, task)

		found, err := tasks.FindByID(ctx, task.ID)
		if err != nil {
			t.Fatalf("find task: %v", err)
		}
		assertTaskEqual(t, found, task)
	})

	t.Run("create self assigned task", func(t *testing.T) {
		task := newRepositoryTask(t, creator.ID, creator.ID, "self assigned", testTaskTime(2))

		created, err := tasks.Create(ctx, task)
		if err != nil {
			t.Fatalf("create self-assigned task: %v", err)
		}
		if created.CreatorID != creator.ID || created.AssigneeID != creator.ID {
			t.Fatalf("self-assigned identities mismatch: %#v", created)
		}
	})

	t.Run("find by id missing", func(t *testing.T) {
		_, err := tasks.FindByID(ctx, uuid.New())
		if !errors.Is(err, taskdomain.ErrTaskNotFound) {
			t.Fatalf("FindByID error = %v, want ErrTaskNotFound", err)
		}
	})

	t.Run("find by id returns archived task", func(t *testing.T) {
		task := newRepositoryTask(t, creator.ID, assignee.ID, "archived find", testTaskTime(3))
		if err := task.Archive(task.CreatedAt.Add(time.Minute)); err != nil {
			t.Fatalf("archive task: %v", err)
		}
		if _, err := tasks.Create(ctx, task); err != nil {
			t.Fatalf("create archived task: %v", err)
		}

		found, err := tasks.FindByID(ctx, task.ID)
		if err != nil {
			t.Fatalf("find archived task: %v", err)
		}
		if found.ArchivedAt == nil {
			t.Fatalf("archived task ArchivedAt = nil")
		}
	})

	t.Run("update title", func(t *testing.T) {
		task := createRepositoryTask(ctx, t, tasks, creator.ID, assignee.ID, "title before", testTaskTime(4))
		updatedAt := task.UpdatedAt.Add(time.Minute)
		if err := task.UpdateTitle("title after", updatedAt); err != nil {
			t.Fatalf("update title domain: %v", err)
		}

		updated, err := tasks.Update(ctx, task)
		if err != nil {
			t.Fatalf("update task: %v", err)
		}

		found, err := tasks.FindByID(ctx, task.ID)
		if err != nil {
			t.Fatalf("find updated task: %v", err)
		}
		if updated.Title != "title after" || found.Title != "title after" || !found.UpdatedAt.Equal(updatedAt) {
			t.Fatalf("title update mismatch: updated=%#v found=%#v", updated, found)
		}
		if found.CreatorID != creator.ID {
			t.Fatalf("CreatorID changed: %s", found.CreatorID)
		}
	})

	t.Run("update valid status", func(t *testing.T) {
		task := createRepositoryTask(ctx, t, tasks, creator.ID, assignee.ID, "status before", testTaskTime(5))
		completedAt := task.UpdatedAt.Add(time.Minute)
		if err := task.ChangeStatus(taskdomain.StatusDone, completedAt); err != nil {
			t.Fatalf("change status domain: %v", err)
		}

		if _, err := tasks.Update(ctx, task); err != nil {
			t.Fatalf("update status: %v", err)
		}

		found, err := tasks.FindByID(ctx, task.ID)
		if err != nil {
			t.Fatalf("find status task: %v", err)
		}
		if found.Status != taskdomain.StatusDone {
			t.Fatalf("Status = %s, want DONE", found.Status)
		}
		if found.CompletedAt == nil || !found.CompletedAt.Equal(completedAt) {
			t.Fatalf("CompletedAt = %v, want %s", found.CompletedAt, completedAt)
		}
	})

	t.Run("archive and restore persistence", func(t *testing.T) {
		task := createRepositoryTask(ctx, t, tasks, creator.ID, assignee.ID, "archive restore", testTaskTime(6))
		archivedAt := task.UpdatedAt.Add(time.Minute)
		if err := task.Archive(archivedAt); err != nil {
			t.Fatalf("archive domain: %v", err)
		}
		if _, err := tasks.Update(ctx, task); err != nil {
			t.Fatalf("update archived task: %v", err)
		}

		found, err := tasks.FindByID(ctx, task.ID)
		if err != nil {
			t.Fatalf("find archived task: %v", err)
		}
		if found.ArchivedAt == nil || !found.ArchivedAt.Equal(archivedAt) {
			t.Fatalf("ArchivedAt = %v, want %s", found.ArchivedAt, archivedAt)
		}

		restoredAt := archivedAt.Add(time.Minute)
		if err := found.Restore(restoredAt); err != nil {
			t.Fatalf("restore domain: %v", err)
		}
		if _, err := tasks.Update(ctx, found); err != nil {
			t.Fatalf("update restored task: %v", err)
		}

		restored, err := tasks.FindByID(ctx, task.ID)
		if err != nil {
			t.Fatalf("find restored task: %v", err)
		}
		if restored.ArchivedAt != nil {
			t.Fatalf("ArchivedAt = %v, want nil", restored.ArchivedAt)
		}
	})

	t.Run("list filters", func(t *testing.T) {
		base := testTaskTime(20)
		activeA := createRepositoryTask(ctx, t, tasks, creator.ID, assignee.ID, "list active alpha", base)
		activeB := createRepositoryTask(ctx, t, tasks, otherCreator.ID, otherAssignee.ID, "list active beta", base.Add(time.Minute))
		createdByCreator := createRepositoryTask(ctx, t, tasks, creator.ID, otherAssignee.ID, "list visible created", base.Add(2*time.Minute))
		assignedToCreator := createRepositoryTask(ctx, t, tasks, otherCreator.ID, creator.ID, "list visible assigned", base.Add(3*time.Minute))
		selfAssignedCreator := createRepositoryTask(ctx, t, tasks, creator.ID, creator.ID, "list visible self", base.Add(4*time.Minute))
		archived := createRepositoryTask(ctx, t, tasks, creator.ID, assignee.ID, "list archived", base.Add(5*time.Minute))
		if err := archived.Archive(archived.UpdatedAt.Add(time.Minute)); err != nil {
			t.Fatalf("archive domain: %v", err)
		}
		if _, err := tasks.Update(ctx, archived); err != nil {
			t.Fatalf("archive update: %v", err)
		}

		defaultList, err := tasks.List(ctx, taskout.TaskFilter{})
		if err != nil {
			t.Fatalf("list default: %v", err)
		}
		assertContainsTask(t, defaultList, activeA.ID)
		assertContainsTask(t, defaultList, activeB.ID)
		assertNotContainsTask(t, defaultList, archived.ID)

		archivedList, err := tasks.List(ctx, taskout.TaskFilter{Archived: taskout.ArchivedOnly})
		if err != nil {
			t.Fatalf("list archived: %v", err)
		}
		assertContainsTask(t, archivedList, archived.ID)
		assertNotContainsTask(t, archivedList, activeA.ID)

		byAssignee, err := tasks.List(ctx, taskout.TaskFilter{AssigneeID: &assignee.ID})
		if err != nil {
			t.Fatalf("list by assignee: %v", err)
		}
		assertContainsTask(t, byAssignee, activeA.ID)
		assertNotContainsTask(t, byAssignee, activeB.ID)

		byCreator, err := tasks.List(ctx, taskout.TaskFilter{CreatorID: &otherCreator.ID})
		if err != nil {
			t.Fatalf("list by creator: %v", err)
		}
		assertContainsTask(t, byCreator, activeB.ID)
		assertNotContainsTask(t, byCreator, activeA.ID)

		visibleToCreator, err := tasks.List(ctx, taskout.TaskFilter{VisibleTo: &creator.ID})
		if err != nil {
			t.Fatalf("list visible to creator: %v", err)
		}
		assertContainsTask(t, visibleToCreator, activeA.ID)
		assertContainsTask(t, visibleToCreator, createdByCreator.ID)
		assertContainsTask(t, visibleToCreator, assignedToCreator.ID)
		assertContainsTaskOnce(t, visibleToCreator, selfAssignedCreator.ID)
		assertNotContainsTask(t, visibleToCreator, activeB.ID)
		assertNotContainsTask(t, visibleToCreator, archived.ID)

		status := taskdomain.StatusOpen
		byStatus, err := tasks.List(ctx, taskout.TaskFilter{Status: &status})
		if err != nil {
			t.Fatalf("list by status: %v", err)
		}
		assertContainsTask(t, byStatus, activeA.ID)
		assertContainsTask(t, byStatus, activeB.ID)
		assertNotContainsTask(t, byStatus, archived.ID)
	})

	t.Run("list deadline range overdue search and sort", func(t *testing.T) {
		base := testTaskTime(40)
		before := createRepositoryTaskWithDeadline(ctx, t, tasks, creator.ID, assignee.ID, "range before", "outside", base, base.Add(time.Hour))
		inside := createRepositoryTaskWithDeadline(ctx, t, tasks, creator.ID, assignee.ID, "range inside needle", "inside description", base.Add(time.Minute), base.Add(3*time.Hour))
		after := createRepositoryTaskWithDeadline(ctx, t, tasks, creator.ID, assignee.ID, "range after", "outside", base.Add(2*time.Minute), base.Add(5*time.Hour))
		noDeadline := createRepositoryTask(ctx, t, tasks, creator.ID, assignee.ID, "range no deadline", base.Add(3*time.Minute))
		completedPast := createRepositoryTaskWithDeadline(ctx, t, tasks, creator.ID, assignee.ID, "range completed past", "needle done", base.Add(4*time.Minute), base.Add(time.Hour))
		if err := completedPast.ChangeStatus(taskdomain.StatusDone, completedPast.UpdatedAt.Add(time.Minute)); err != nil {
			t.Fatalf("complete domain: %v", err)
		}
		if _, err := tasks.Update(ctx, completedPast); err != nil {
			t.Fatalf("complete update: %v", err)
		}

		from := base.Add(2 * time.Hour)
		to := base.Add(4 * time.Hour)
		inRange, err := tasks.List(ctx, taskout.TaskFilter{
			DeadlineFrom: &from,
			DeadlineTo:   &to,
			Sort:         taskout.TaskSortDeadlineAtAsc,
		})
		if err != nil {
			t.Fatalf("list deadline range: %v", err)
		}
		assertContainsTask(t, inRange, inside.ID)
		assertNotContainsTask(t, inRange, before.ID)
		assertNotContainsTask(t, inRange, after.ID)
		assertNotContainsTask(t, inRange, noDeadline.ID)

		overdue := true
		now := base.Add(4 * time.Hour)
		overdueTasks, err := tasks.List(ctx, taskout.TaskFilter{Overdue: &overdue, Now: &now})
		if err != nil {
			t.Fatalf("list overdue: %v", err)
		}
		assertContainsTask(t, overdueTasks, before.ID)
		assertContainsTask(t, overdueTasks, inside.ID)
		assertNotContainsTask(t, overdueTasks, after.ID)
		assertNotContainsTask(t, overdueTasks, noDeadline.ID)
		assertNotContainsTask(t, overdueTasks, completedPast.ID)

		search, err := tasks.List(ctx, taskout.TaskFilter{Search: "NEEDLE", Archived: taskout.ArchivedAll})
		if err != nil {
			t.Fatalf("list search: %v", err)
		}
		assertContainsTask(t, search, inside.ID)
		assertContainsTask(t, search, completedPast.ID)
		assertNotContainsTask(t, search, before.ID)
	})

	t.Run("invalid persisted status returns mapping error", func(t *testing.T) {
		id := uuid.New()
		if _, err := pg.SQL.ExecContext(ctx, `
			INSERT INTO tasks (id, creator_id, assignee_id, title, description, status, created_at, updated_at)
			VALUES ($1, $2, $3, 'invalid status', '', 'BROKEN', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, id, creator.ID, assignee.ID); err != nil {
			t.Fatalf("insert invalid status task: %v", err)
		}

		_, err := tasks.FindByID(ctx, id)
		if !errors.Is(err, taskdomain.ErrInvalidStatus) {
			t.Fatalf("FindByID invalid status error = %v, want ErrInvalidStatus", err)
		}
	})

	t.Run("uses caller context", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := tasks.FindByID(canceledCtx, uuid.New())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("find with canceled context error = %v, want context.Canceled", err)
		}
	})
}

func createTaskSeriesFixture(ctx context.Context, t *testing.T, db *sql.DB, creatorID uuid.UUID, assigneeID uuid.UUID) uuid.UUID {
	t.Helper()

	seriesID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO task_series (
			id,
			creator_id,
			assignee_id,
			title,
			description,
			frequency,
			"interval",
			next_deadline_at,
			timezone,
			is_active,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, 'task repository fixture', '', 'DAILY', 1, $4, 'UTC', true, $4, $4)
	`, seriesID, creatorID, assigneeID, testTaskTime(1))
	if err != nil {
		t.Fatalf("create task series fixture: %v", err)
	}

	return seriesID
}

func createTaskUser(ctx context.Context, t *testing.T, repo *userpostgres.Repository, email string, username string) userdomain.User {
	t.Helper()

	user := newTestUser(email, username)
	created, err := repo.Create(ctx, user)
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}

	return created
}

func newRepositoryTask(t *testing.T, creatorID uuid.UUID, assigneeID uuid.UUID, title string, createdAt time.Time) taskdomain.Task {
	t.Helper()

	task, err := taskdomain.NewTask(creatorID, assigneeID, title, "", nil, createdAt)
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	return task
}

func createRepositoryTask(
	ctx context.Context,
	t *testing.T,
	repo *taskpostgres.Repository,
	creatorID uuid.UUID,
	assigneeID uuid.UUID,
	title string,
	createdAt time.Time,
) taskdomain.Task {
	t.Helper()

	task := newRepositoryTask(t, creatorID, assigneeID, title, createdAt)
	created, err := repo.Create(ctx, task)
	if err != nil {
		t.Fatalf("create task %s: %v", title, err)
	}

	return created
}

func createRepositoryTaskWithDeadline(
	ctx context.Context,
	t *testing.T,
	repo *taskpostgres.Repository,
	creatorID uuid.UUID,
	assigneeID uuid.UUID,
	title string,
	description string,
	createdAt time.Time,
	deadlineAt time.Time,
) taskdomain.Task {
	t.Helper()

	task := newRepositoryTask(t, creatorID, assigneeID, title, createdAt)
	task.Description = description
	task.DeadlineAt = &deadlineAt
	created, err := repo.Create(ctx, task)
	if err != nil {
		t.Fatalf("create task %s: %v", title, err)
	}

	return created
}

func assertTaskEqual(t *testing.T, got taskdomain.Task, want taskdomain.Task) {
	t.Helper()

	if got.ID != want.ID ||
		got.CreatorID != want.CreatorID ||
		got.AssigneeID != want.AssigneeID ||
		got.Title != want.Title ||
		got.Description != want.Description ||
		got.Status != want.Status ||
		!got.CreatedAt.Equal(want.CreatedAt) ||
		!got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("task mismatch:\ngot  %#v\nwant %#v", got, want)
	}
	assertUUIDPtrEqual(t, got.SeriesID, want.SeriesID)
	assertTimePtrEqual(t, got.DeadlineAt, want.DeadlineAt)
	assertTimePtrEqual(t, got.CompletedAt, want.CompletedAt)
	assertTimePtrEqual(t, got.ArchivedAt, want.ArchivedAt)
}

func assertContainsTask(t *testing.T, tasks []taskdomain.Task, id uuid.UUID) {
	t.Helper()

	for _, task := range tasks {
		if task.ID == id {
			return
		}
	}

	t.Fatalf("expected task %s in result %s", id, taskIDs(tasks))
}

func assertContainsTaskOnce(t *testing.T, tasks []taskdomain.Task, id uuid.UUID) {
	t.Helper()

	count := 0
	for _, task := range tasks {
		if task.ID == id {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected task %s once in result %s, got %d", id, taskIDs(tasks), count)
	}
}

func assertNotContainsTask(t *testing.T, tasks []taskdomain.Task, id uuid.UUID) {
	t.Helper()

	for _, task := range tasks {
		if task.ID == id {
			t.Fatalf("did not expect task %s in result %s", id, taskIDs(tasks))
		}
	}
}

func taskIDs(tasks []taskdomain.Task) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}

	return ids
}

func testTaskTime(offsetMinutes int) time.Time {
	return time.Date(2026, 9, 14, 12, offsetMinutes, 0, 123456000, time.UTC)
}

func assertUUIDPtrEqual(t *testing.T, got *uuid.UUID, want *uuid.UUID) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("uuid pointer mismatch: got %v want %v", got, want)
	case *got != *want:
		t.Fatalf("uuid pointer = %s, want %s", *got, *want)
	}
}

func assertTimePtrEqual(t *testing.T, got *time.Time, want *time.Time) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("time pointer mismatch: got %v want %v", got, want)
	case !got.Equal(*want):
		t.Fatalf("time pointer = %s, want %s", *got, *want)
	}
}
