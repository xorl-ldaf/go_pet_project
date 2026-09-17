package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/migrations"
	recurrenceservice "go_pet_project/internal/recurrence/application/service"
	recurrencedomain "go_pet_project/internal/recurrence/domain"
	reminderpostgres "go_pet_project/internal/reminder/adapter/out/postgres"
	reminderdomain "go_pet_project/internal/reminder/domain"
	taskpostgres "go_pet_project/internal/task/adapter/out/postgres"
	taskdomain "go_pet_project/internal/task/domain"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

const recurrenceMigrationsDir = "../../../../../migrations"

var (
	recurrenceTestInfraOnce      sync.Once
	recurrenceTestInfraStartErr  error
	recurrenceTestInfraSkipCause string
)

func TestRepositoryCreateFindReminderRulesAndClaimDue(t *testing.T) {
	ctx, pg := setupRecurrencePostgres(t)
	repo := NewRepository(pg.GORM)
	creatorID, assigneeID := seedRecurrenceUsers(ctx, t, pg.SQL)
	now := recurrenceTestTime()

	dueOne := createTestSeries(ctx, t, repo, creatorID, assigneeID, now.Add(-time.Hour), true)
	dueTwo := createTestSeries(ctx, t, repo, creatorID, assigneeID, now.Add(-30*time.Minute), true)
	createTestSeries(ctx, t, repo, creatorID, assigneeID, now.Add(-2*time.Hour), false)
	createTestSeries(ctx, t, repo, creatorID, assigneeID, now.Add(time.Hour), true)

	rule, err := recurrencedomain.NewReminderRule(dueOne.ID, 3600)
	if err != nil {
		t.Fatalf("NewReminderRule: %v", err)
	}
	if _, err := repo.CreateReminderRule(ctx, rule); err != nil {
		t.Fatalf("CreateReminderRule: %v", err)
	}

	found, err := repo.FindByID(ctx, dueOne.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.ID != dueOne.ID || found.Title != dueOne.Title {
		t.Fatalf("found series = %+v, want %+v", found, dueOne)
	}

	rules, err := repo.ListReminderRules(ctx, dueOne.ID)
	if err != nil {
		t.Fatalf("ListReminderRules: %v", err)
	}
	if len(rules) != 1 || rules[0].OffsetSeconds != 3600 {
		t.Fatalf("rules = %+v, want one 3600s rule", rules)
	}

	claimed, err := repo.ClaimDue(ctx, now, 1, func(_ context.Context, series []recurrencedomain.TaskSeries) error {
		if len(series) != 1 {
			t.Fatalf("claimed batch len = %d, want 1", len(series))
		}
		if series[0].ID != dueOne.ID {
			t.Fatalf("claimed first id = %s, want %s", series[0].ID, dueOne.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("claimed = %d, want 1", claimed)
	}

	claimed, err = repo.ClaimDue(ctx, now, 10, func(_ context.Context, series []recurrencedomain.TaskSeries) error {
		ids := []uuid.UUID{series[0].ID, series[1].ID}
		sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
		want := []uuid.UUID{dueOne.ID, dueTwo.ID}
		sort.Slice(want, func(i, j int) bool { return want[i].String() < want[j].String() })
		if ids[0] != want[0] || ids[1] != want[1] {
			t.Fatalf("claimed ids = %v, want %v", ids, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ClaimDue all: %v", err)
	}
	if claimed != 2 {
		t.Fatalf("claimed = %d, want 2", claimed)
	}
}

func TestRepositoryClaimDueSkipLocked(t *testing.T) {
	ctx, pg := setupRecurrencePostgres(t)
	repo := NewRepository(pg.GORM)
	creatorID, assigneeID := seedRecurrenceUsers(ctx, t, pg.SQL)
	now := recurrenceTestTime()
	createTestSeries(ctx, t, repo, creatorID, assigneeID, now.Add(-time.Hour), true)

	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := repo.ClaimDue(ctx, now, 1, func(_ context.Context, _ []recurrencedomain.TaskSeries) error {
			close(locked)
			<-release
			return nil
		})
		done <- err
	}()

	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("first claim did not lock series")
	}

	claimed, err := repo.ClaimDue(ctx, now, 1, func(_ context.Context, series []recurrencedomain.TaskSeries) error {
		if len(series) != 0 {
			t.Fatalf("second claim got locked series: %+v", series)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("second ClaimDue: %v", err)
	}
	if claimed != 0 {
		t.Fatalf("second claimed = %d, want 0", claimed)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first ClaimDue: %v", err)
	}
}

func TestGenerationCreatesOneTaskRemindersAndAdvancesSeries(t *testing.T) {
	ctx, pg := setupRecurrencePostgres(t)
	seriesRepo, taskRepo, reminderRepo, processor := setupRecurrenceProcessor(t, pg, nil)
	creatorID, assigneeID := seedRecurrenceUsers(ctx, t, pg.SQL)
	deadline := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	series := createTestSeries(ctx, t, seriesRepo, creatorID, assigneeID, deadline, true)
	createTestRule(ctx, t, seriesRepo, series.ID, 3600)
	createTestRule(ctx, t, seriesRepo, series.ID, 7200)

	processed, err := processor.ProcessDue(ctx, deadline.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("ProcessDue: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}

	tasks := listTasksBySeries(ctx, t, pg.SQL, series.ID)
	if len(tasks) != 1 {
		t.Fatalf("tasks len = %d, want 1", len(tasks))
	}
	task := tasks[0]
	if task.seriesID == nil || *task.seriesID != series.ID {
		t.Fatalf("task series id = %v, want %s", task.seriesID, series.ID)
	}
	if task.creatorID != creatorID || task.assigneeID != assigneeID || task.title != series.Title || task.description != series.Description {
		t.Fatalf("task fields not copied: %+v", task)
	}
	if task.status != string(taskdomain.InitialStatus) {
		t.Fatalf("task status = %s, want %s", task.status, taskdomain.InitialStatus)
	}
	if task.deadlineAt == nil || !task.deadlineAt.Equal(deadline) {
		t.Fatalf("task deadline = %v, want %s", task.deadlineAt, deadline)
	}

	reminders, err := reminderRepo.ListByTaskID(ctx, task.id)
	if err != nil {
		t.Fatalf("ListByTaskID: %v", err)
	}
	if len(reminders) != 2 {
		t.Fatalf("reminders len = %d, want 2", len(reminders))
	}
	assertReminder(t, reminders[0], task.id, deadline.Add(-2*time.Hour), 7200)
	assertReminder(t, reminders[1], task.id, deadline.Add(-time.Hour), 3600)

	advanced, err := seriesRepo.FindByID(ctx, series.ID)
	if err != nil {
		t.Fatalf("FindByID advanced: %v", err)
	}
	if !advanced.NextDeadlineAt.Equal(deadline.AddDate(0, 0, 1)) {
		t.Fatalf("next deadline = %s, want %s", advanced.NextDeadlineAt, deadline.AddDate(0, 0, 1))
	}
	if !advanced.IsActive {
		t.Fatal("series inactive, want active")
	}

	_ = taskRepo
}

func TestGenerationRollbackOnReminderFailure(t *testing.T) {
	ctx, pg := setupRecurrencePostgres(t)
	failingReminders := failingReminderWriter{err: errors.New("forced reminder failure")}
	seriesRepo, _, _, processor := setupRecurrenceProcessor(t, pg, failingReminders)
	creatorID, assigneeID := seedRecurrenceUsers(ctx, t, pg.SQL)
	deadline := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	series := createTestSeries(ctx, t, seriesRepo, creatorID, assigneeID, deadline, true)
	createTestRule(ctx, t, seriesRepo, series.ID, 3600)

	if _, err := processor.ProcessDue(ctx, deadline.Add(time.Hour), 10); err == nil {
		t.Fatal("ProcessDue error = nil, want forced failure")
	}

	if countTasksBySeries(ctx, t, pg.SQL, series.ID) != 0 {
		t.Fatal("task persisted after rollback")
	}
	if countRemindersBySeries(ctx, t, pg.SQL, series.ID) != 0 {
		t.Fatal("reminder persisted after rollback")
	}
	found, err := seriesRepo.FindByID(ctx, series.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.NextDeadlineAt.Equal(deadline) || !found.IsActive {
		t.Fatalf("series changed after rollback: %+v", found)
	}
}

func TestConcurrentGenerationCreatesOneOccurrenceAndUniqueConstraintRejectsDuplicate(t *testing.T) {
	ctx, pg := setupRecurrencePostgres(t)
	blocking := newBlockingReminderWriter(reminderpostgres.NewRepository(pg.GORM))
	seriesRepo, taskRepo, _, processorOne := setupRecurrenceProcessor(t, pg, blocking)
	_, _, _, processorTwo := setupRecurrenceProcessor(t, pg, reminderpostgres.NewRepository(pg.GORM))
	creatorID, assigneeID := seedRecurrenceUsers(ctx, t, pg.SQL)
	deadline := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	series := createTestSeries(ctx, t, seriesRepo, creatorID, assigneeID, deadline, true)
	createTestRule(ctx, t, seriesRepo, series.ID, 3600)

	firstDone := make(chan error, 1)
	go func() {
		_, err := processorOne.ProcessDue(ctx, deadline.Add(time.Hour), 1)
		firstDone <- err
	}()

	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first processor did not reach reminder creation")
	}

	processed, err := processorTwo.ProcessDue(ctx, deadline.Add(time.Hour), 1)
	if err != nil {
		t.Fatalf("second ProcessDue: %v", err)
	}
	if processed != 0 {
		t.Fatalf("second processed = %d, want 0 while row locked", processed)
	}

	close(blocking.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first ProcessDue: %v", err)
	}
	if countTasksBySeries(ctx, t, pg.SQL, series.ID) != 1 {
		t.Fatalf("tasks count = %d, want 1", countTasksBySeries(ctx, t, pg.SQL, series.ID))
	}

	duplicate, err := taskdomain.NewTaskOccurrence(series.ID, creatorID, assigneeID, series.Title, series.Description, deadline, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewTaskOccurrence duplicate: %v", err)
	}
	if _, err := taskRepo.Create(ctx, duplicate); err == nil {
		t.Fatal("duplicate occurrence insert error = nil, want unique constraint failure")
	}
}

func TestCatchUpProcessesOneOccurrencePerPoll(t *testing.T) {
	ctx, pg := setupRecurrencePostgres(t)
	seriesRepo, _, _, processor := setupRecurrenceProcessor(t, pg, nil)
	creatorID, assigneeID := seedRecurrenceUsers(ctx, t, pg.SQL)
	firstDeadline := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	series := createTestSeries(ctx, t, seriesRepo, creatorID, assigneeID, firstDeadline, true)
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		processed, err := processor.ProcessDue(ctx, now, 10)
		if err != nil {
			t.Fatalf("ProcessDue #%d: %v", i+1, err)
		}
		if processed != 1 {
			t.Fatalf("processed #%d = %d, want 1", i+1, processed)
		}
	}

	tasks := listTasksBySeries(ctx, t, pg.SQL, series.ID)
	if len(tasks) != 3 {
		t.Fatalf("tasks len = %d, want 3", len(tasks))
	}
	for i, task := range tasks {
		want := firstDeadline.AddDate(0, 0, i)
		if task.deadlineAt == nil || !task.deadlineAt.Equal(want) {
			t.Fatalf("task #%d deadline = %v, want %s", i+1, task.deadlineAt, want)
		}
	}
}

type testReminderWriter interface {
	Create(context.Context, reminderdomain.Reminder) (reminderdomain.Reminder, error)
}

func setupRecurrenceProcessor(
	t *testing.T,
	pg *database.Postgres,
	writer testReminderWriter,
) (*Repository, *taskpostgres.Repository, *reminderpostgres.Repository, *recurrenceservice.Processor) {
	t.Helper()

	seriesRepo := NewRepository(pg.GORM)
	taskRepo := taskpostgres.NewRepository(pg.GORM)
	reminderRepo := reminderpostgres.NewRepository(pg.GORM)
	if writer == nil {
		writer = reminderRepo
	}

	processor, err := recurrenceservice.NewProcessor(
		seriesRepo,
		taskRepo,
		writer,
		database.NewTransactionRunner(pg.GORM),
	)
	if err != nil {
		t.Fatalf("NewProcessor: %v", err)
	}

	return seriesRepo, taskRepo, reminderRepo, processor
}

func setupRecurrencePostgres(t *testing.T) (context.Context, *database.Postgres) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	if os.Getenv("JWT_SECRET") == "" {
		t.Setenv("JWT_SECRET", "test-jwt-secret-with-at-least-32-bytes")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	startRecurrencePostgres(ctx, t)
	waitForRecurrencePostgres(ctx, t, cfg.DB)

	testDBName := fmt.Sprintf("todo_recurrence_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	adminDB := openRecurrenceAdminDB(ctx, t, cfg.DB)
	createRecurrenceDatabase(ctx, t, adminDB, testDBName)
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer dropCancel()
		dropRecurrenceDatabase(dropCtx, t, adminDB, testDBName)
		adminDB.Close()
	})

	testCfg := cfg.DB
	testCfg.Name = testDBName
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrations.Up(ctx, testCfg, recurrenceMigrationsDir, logger); err != nil {
		t.Fatalf("migration up: %v", err)
	}

	pg, err := database.Open(ctx, testCfg, logger)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := pg.Close(); err != nil {
			t.Logf("close postgres: %v", err)
		}
	})

	return ctx, pg
}

func seedRecurrenceUsers(ctx context.Context, t *testing.T, db *sql.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()

	creatorID := uuid.New()
	assigneeID := uuid.New()
	insertUser(ctx, t, db, creatorID, "creator")
	insertUser(ctx, t, db, assigneeID, "assignee")

	return creatorID, assigneeID
}

func insertUser(ctx context.Context, t *testing.T, db *sql.DB, id uuid.UUID, suffix string) {
	t.Helper()

	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, timezone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'UTC', now(), now())
	`, id, fmt.Sprintf("%s-%s@example.com", suffix, id), fmt.Sprintf("%s_%s", suffix, strings.ReplaceAll(id.String(), "-", "_")), "hash")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func createTestSeries(ctx context.Context, t *testing.T, repo *Repository, creatorID uuid.UUID, assigneeID uuid.UUID, deadline time.Time, active bool) recurrencedomain.TaskSeries {
	t.Helper()

	now := recurrenceTestTime().Add(-24 * time.Hour)
	series, err := recurrencedomain.RestoreTaskSeries(
		uuid.New(),
		creatorID,
		assigneeID,
		"recurring task",
		"recurring description",
		recurrencedomain.FrequencyDaily,
		1,
		deadline,
		"UTC",
		nil,
		active,
		now,
		now,
	)
	if err != nil {
		t.Fatalf("RestoreTaskSeries: %v", err)
	}
	created, err := repo.Create(ctx, series)
	if err != nil {
		t.Fatalf("Create series: %v", err)
	}

	return created
}

func createTestRule(ctx context.Context, t *testing.T, repo *Repository, seriesID uuid.UUID, offsetSeconds int64) {
	t.Helper()

	rule, err := recurrencedomain.NewReminderRule(seriesID, offsetSeconds)
	if err != nil {
		t.Fatalf("NewReminderRule: %v", err)
	}
	if _, err := repo.CreateReminderRule(ctx, rule); err != nil {
		t.Fatalf("CreateReminderRule: %v", err)
	}
}

type taskRow struct {
	id          uuid.UUID
	seriesID    *uuid.UUID
	creatorID   uuid.UUID
	assigneeID  uuid.UUID
	title       string
	description string
	status      string
	deadlineAt  *time.Time
}

func listTasksBySeries(ctx context.Context, t *testing.T, db *sql.DB, seriesID uuid.UUID) []taskRow {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
		SELECT id, series_id, creator_id, assignee_id, title, description, status, deadline_at
		FROM tasks
		WHERE series_id = $1
		ORDER BY deadline_at ASC
	`, seriesID)
	if err != nil {
		t.Fatalf("query tasks by series: %v", err)
	}
	defer rows.Close()

	var tasks []taskRow
	for rows.Next() {
		var row taskRow
		var series uuid.UUID
		var deadline time.Time
		if err := rows.Scan(&row.id, &series, &row.creatorID, &row.assigneeID, &row.title, &row.description, &row.status, &deadline); err != nil {
			t.Fatalf("scan task row: %v", err)
		}
		row.seriesID = &series
		row.deadlineAt = &deadline
		tasks = append(tasks, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate task rows: %v", err)
	}

	return tasks
}

func countTasksBySeries(ctx context.Context, t *testing.T, db *sql.DB, seriesID uuid.UUID) int {
	t.Helper()

	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE series_id = $1`, seriesID).Scan(&count); err != nil {
		t.Fatalf("count tasks by series: %v", err)
	}

	return count
}

func countRemindersBySeries(ctx context.Context, t *testing.T, db *sql.DB, seriesID uuid.UUID) int {
	t.Helper()

	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM reminders r
		JOIN tasks t ON t.id = r.task_id
		WHERE t.series_id = $1
	`, seriesID).Scan(&count); err != nil {
		t.Fatalf("count reminders by series: %v", err)
	}

	return count
}

func assertReminder(t *testing.T, reminder reminderdomain.Reminder, taskID uuid.UUID, triggerAt time.Time, offsetSeconds int64) {
	t.Helper()

	if reminder.TaskID != taskID {
		t.Fatalf("reminder task id = %s, want %s", reminder.TaskID, taskID)
	}
	if reminder.Kind != reminderdomain.KindBeforeDeadline {
		t.Fatalf("reminder kind = %s, want BEFORE_DEADLINE", reminder.Kind)
	}
	if reminder.State != reminderdomain.InitialState {
		t.Fatalf("reminder state = %s, want %s", reminder.State, reminderdomain.InitialState)
	}
	if reminder.OffsetSeconds == nil || *reminder.OffsetSeconds != offsetSeconds {
		t.Fatalf("offset = %v, want %d", reminder.OffsetSeconds, offsetSeconds)
	}
	if !reminder.TriggerAt.Equal(triggerAt) {
		t.Fatalf("trigger_at = %s, want %s", reminder.TriggerAt, triggerAt)
	}
}

type failingReminderWriter struct {
	err error
}

func (w failingReminderWriter) Create(context.Context, reminderdomain.Reminder) (reminderdomain.Reminder, error) {
	return reminderdomain.Reminder{}, w.err
}

type blockingReminderWriter struct {
	inner   *reminderpostgres.Repository
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingReminderWriter(inner *reminderpostgres.Repository) *blockingReminderWriter {
	return &blockingReminderWriter{
		inner:   inner,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (w *blockingReminderWriter) Create(ctx context.Context, reminder reminderdomain.Reminder) (reminderdomain.Reminder, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})

	return w.inner.Create(ctx, reminder)
}

func recurrenceTestTime() time.Time {
	return time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
}

func startRecurrencePostgres(ctx context.Context, t *testing.T) {
	t.Helper()

	if usesExternalRecurrenceTestInfrastructure() {
		return
	}

	recurrenceTestInfraOnce.Do(func() {
		recurrenceTestInfraSkipCause, recurrenceTestInfraStartErr = startLocalRecurrencePostgres(ctx)
	})
	if recurrenceTestInfraSkipCause != "" {
		t.Skip(recurrenceTestInfraSkipCause)
	}
	if recurrenceTestInfraStartErr != nil {
		t.Fatal(recurrenceTestInfraStartErr)
	}
}

func usesExternalRecurrenceTestInfrastructure() bool {
	return strings.EqualFold(os.Getenv("TEST_INFRA_EXTERNAL"), "true")
}

func startLocalRecurrencePostgres(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "Docker CLI is required for postgres recurrence tests", nil
	}

	infoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(infoCtx, "docker", "info").CombinedOutput(); err != nil {
		return fmt.Sprintf("Docker daemon is required for postgres recurrence tests: %s", strings.TrimSpace(string(output))), nil
	}

	composeCtx, composeCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer composeCancel()
	cmd := exec.CommandContext(composeCtx, "docker", "compose", "-f", "../../../../../deploy/compose.yaml", "up", "-d", "postgres")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker compose up: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return "", nil
}

func waitForRecurrencePostgres(ctx context.Context, t *testing.T, cfg config.DBConfig) {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for {
		db, err := sql.Open("postgres", cfg.URL())
		if err == nil {
			pingErr := db.PingContext(ctx)
			db.Close()
			if pingErr == nil {
				return
			}
			err = pingErr
		}
		if time.Now().After(deadline) {
			t.Fatalf("postgres did not become ready: %v", err)
		}

		select {
		case <-ctx.Done():
			t.Fatalf("wait postgres: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func openRecurrenceAdminDB(ctx context.Context, t *testing.T, cfg config.DBConfig) *sql.DB {
	t.Helper()

	adminCfg := cfg
	adminCfg.Name = "postgres"
	db, err := sql.Open("postgres", adminCfg.URL())
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("ping admin database: %v", err)
	}

	return db
}

func createRecurrenceDatabase(ctx context.Context, t *testing.T, db *sql.DB, name string) {
	t.Helper()

	if _, err := db.ExecContext(ctx, `CREATE DATABASE `+quoteRecurrenceIdent(name)); err != nil {
		t.Fatalf("create test database: %v", err)
	}
}

func dropRecurrenceDatabase(ctx context.Context, t *testing.T, db *sql.DB, name string) {
	t.Helper()

	if _, err := db.ExecContext(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()
	`, name); err != nil {
		t.Logf("terminate test database connections: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP DATABASE IF EXISTS `+quoteRecurrenceIdent(name)); err != nil {
		t.Logf("drop test database: %v", err)
	}
}

func quoteRecurrenceIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
