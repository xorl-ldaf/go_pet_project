package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	reminderout "go_pet_project/internal/reminder/application/port/out"
	"go_pet_project/internal/reminder/domain"

	"github.com/google/uuid"
)

func TestRunnerRunExitsOnContextCancellation(t *testing.T) {
	claimer := &fakeDueReminderClaimer{}
	runner := newTestRunner(t, claimer, NewNoopProcessor(), Config{
		Interval:  time.Hour,
		BatchSize: 10,
		Workers:   1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt64(&claimer.calls) == 0 {
		select {
		case err := <-done:
			t.Fatalf("Run exited before first poll: %v", err)
		case <-deadline:
			t.Fatal("Run did not execute immediate poll")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}
}

func TestRunnerWorkerBound(t *testing.T) {
	const workerLimit = 2
	reminders := []domain.Reminder{
		mustSchedulerTestReminder(t, 1),
		mustSchedulerTestReminder(t, 2),
		mustSchedulerTestReminder(t, 3),
		mustSchedulerTestReminder(t, 4),
		mustSchedulerTestReminder(t, 5),
	}
	claimer := &fakeDueReminderClaimer{reminders: reminders}

	var current int64
	var maxSeen int64
	processor := ProcessorFunc(func(ctx context.Context, _ domain.Reminder) error {
		running := atomic.AddInt64(&current, 1)
		for {
			previous := atomic.LoadInt64(&maxSeen)
			if running <= previous || atomic.CompareAndSwapInt64(&maxSeen, previous, running) {
				break
			}
		}
		defer atomic.AddInt64(&current, -1)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
			return nil
		}
	})

	runner := newTestRunner(t, claimer, processor, Config{
		Interval:  time.Hour,
		BatchSize: len(reminders),
		Workers:   workerLimit,
	})

	processed, err := runner.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if processed != len(reminders) {
		t.Fatalf("processed = %d, want %d", processed, len(reminders))
	}
	if maxSeen := atomic.LoadInt64(&maxSeen); maxSeen > workerLimit {
		t.Fatalf("max concurrent processors = %d, want <= %d", maxSeen, workerLimit)
	}
}

func newTestRunner(t *testing.T, claimer reminderout.DueReminderClaimer, processor Processor, cfg Config) *Runner {
	t.Helper()

	runner, err := NewRunner(claimer, processor, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	runner.now = func() time.Time {
		return schedulerTestTime()
	}

	return runner
}

type fakeDueReminderClaimer struct {
	reminders []domain.Reminder
	calls     int64
}

func (c *fakeDueReminderClaimer) ClaimDue(ctx context.Context, _ time.Time, limit int, handle reminderout.DueReminderHandler) (int, error) {
	atomic.AddInt64(&c.calls, 1)

	reminders := c.reminders
	if len(reminders) > limit {
		reminders = reminders[:limit]
	}
	if len(reminders) == 0 {
		return 0, nil
	}
	if err := handle(ctx, reminders); err != nil {
		return 0, err
	}

	return len(reminders), nil
}

func mustSchedulerTestReminder(t *testing.T, suffix byte) domain.Reminder {
	t.Helper()

	reminder, err := domain.NewAbsoluteReminder(uuid.UUID{15: suffix}, uuid.UUID{14: 1, 15: suffix}, schedulerTestTime(), schedulerTestTime().Add(-time.Hour))
	if err != nil {
		t.Fatalf("NewAbsoluteReminder: %v", err)
	}

	return reminder
}

func schedulerTestTime() time.Time {
	return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
}
