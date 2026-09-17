package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"go_pet_project/internal/platform/metrics"
	recurrenceservice "go_pet_project/internal/recurrence/application/service"
)

type Config struct {
	Interval  time.Duration
	BatchSize int
}

type Runner struct {
	processor *recurrenceservice.Processor
	logger    *slog.Logger
	interval  time.Duration
	batchSize int
	now       func() time.Time
}

func NewRunner(processor *recurrenceservice.Processor, cfg Config, logger *slog.Logger) (*Runner, error) {
	if processor == nil {
		return nil, errors.New("recurrence processor is required")
	}
	if cfg.Interval <= 0 {
		return nil, errors.New("recurrence interval must be positive")
	}
	if cfg.BatchSize <= 0 {
		return nil, errors.New("recurrence batch size must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Runner{
		processor: processor,
		logger:    logger.With("component", "recurrence_scheduler"),
		interval:  cfg.Interval,
		batchSize: cfg.BatchSize,
		now:       time.Now,
	}, nil
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
	return r.processor.ProcessDue(ctx, r.now().UTC(), r.batchSize)
}

func (r *Runner) processAndLog(ctx context.Context) error {
	startedAt := time.Now()
	processed, err := r.ProcessOnce(ctx)
	duration := time.Since(startedAt)
	if err != nil {
		metrics.ObserveRecurrenceGenerationFailure()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		r.logger.Error("recurrence scheduler poll failed",
			"batch_size", r.batchSize,
			"processed", processed,
			"duration", duration.String(),
			"error", err,
		)
		return nil
	}

	metrics.ObserveRecurrenceGenerated(processed)
	r.logger.Info("recurrence scheduler poll completed",
		"batch_size", r.batchSize,
		"processed", processed,
		"duration", duration.String(),
	)

	return nil
}
