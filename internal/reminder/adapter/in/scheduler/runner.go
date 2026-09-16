package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/metrics"
	reminderout "go_pet_project/internal/reminder/application/port/out"
	"go_pet_project/internal/reminder/domain"

	"golang.org/x/sync/errgroup"
)

type Processor interface {
	Process(ctx context.Context, reminder domain.Reminder) error
}

type ProcessorFunc func(ctx context.Context, reminder domain.Reminder) error

func (f ProcessorFunc) Process(ctx context.Context, reminder domain.Reminder) error {
	return f(ctx, reminder)
}

type Config struct {
	Interval  time.Duration
	BatchSize int
	Workers   int
}

type Runner struct {
	claimer   reminderout.DueReminderClaimer
	processor Processor
	logger    *slog.Logger
	interval  time.Duration
	batchSize int
	workers   int
	now       func() time.Time
}

func NewRunner(claimer reminderout.DueReminderClaimer, processor Processor, cfg Config, logger *slog.Logger) (*Runner, error) {
	if claimer == nil {
		return nil, errors.New("due reminder claimer is required")
	}
	if processor == nil {
		return nil, errors.New("reminder processor is required")
	}
	if cfg.Interval <= 0 {
		return nil, errors.New("scheduler interval must be positive")
	}
	if cfg.BatchSize <= 0 {
		return nil, errors.New("scheduler batch size must be positive")
	}
	if cfg.Workers <= 0 {
		return nil, errors.New("scheduler workers must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Runner{
		claimer:   claimer,
		processor: processor,
		logger:    logger.With("component", "reminder_scheduler"),
		interval:  cfg.Interval,
		batchSize: cfg.BatchSize,
		workers:   cfg.Workers,
		now:       time.Now,
	}, nil
}

func NewNoopProcessor() Processor {
	return ProcessorFunc(func(context.Context, domain.Reminder) error {
		return nil
	})
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.processAndLog(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.processAndLog(ctx); err != nil {
				return err
			}
		}
	}
}

func (r *Runner) ProcessOnce(ctx context.Context) (int, error) {
	now := r.now().UTC()

	return r.claimer.ClaimDue(ctx, now, r.batchSize, func(txCtx context.Context, reminders []domain.Reminder) error {
		return r.processBatch(txCtx, reminders)
	})
}

func (r *Runner) processAndLog(ctx context.Context) error {
	startedAt := time.Now()
	processed, err := r.ProcessOnce(ctx)
	duration := time.Since(startedAt)
	if err != nil {
		metrics.ObserveReminderSchedulerBatch(processed, duration, true)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		r.logger.Error("reminder scheduler poll failed",
			"batch_size", r.batchSize,
			"processed", processed,
			"duration", duration.String(),
			"error", err,
		)
		return nil
	}

	metrics.ObserveReminderSchedulerBatch(processed, duration, false)
	r.logger.Info("reminder scheduler poll completed",
		"batch_size", r.batchSize,
		"processed", processed,
		"duration", duration.String(),
	)

	return nil
}

func (r *Runner) processBatch(ctx context.Context, reminders []domain.Reminder) error {
	if len(reminders) == 0 {
		return nil
	}
	if r.workers == 1 || len(reminders) == 1 || database.HasGORM(ctx) {
		for _, reminder := range reminders {
			if err := r.processor.Process(ctx, reminder); err != nil {
				return err
			}
		}

		return nil
	}

	workerCount := r.workers
	if workerCount > len(reminders) {
		workerCount = len(reminders)
	}

	group, workerCtx := errgroup.WithContext(ctx)
	jobs := make(chan domain.Reminder)
	for i := 0; i < workerCount; i++ {
		group.Go(func() error {
			for {
				select {
				case <-workerCtx.Done():
					return workerCtx.Err()
				case reminder, ok := <-jobs:
					if !ok {
						return nil
					}
					if err := r.processor.Process(workerCtx, reminder); err != nil {
						return err
					}
				}
			}
		})
	}

	for _, reminder := range reminders {
		select {
		case <-workerCtx.Done():
			close(jobs)
			return group.Wait()
		case jobs <- reminder:
		}
	}
	close(jobs)

	return group.Wait()
}
