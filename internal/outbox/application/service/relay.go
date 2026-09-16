package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	outboxout "go_pet_project/internal/outbox/application/port/out"
	"go_pet_project/internal/outbox/domain"
	"go_pet_project/internal/platform/metrics"

	"golang.org/x/sync/errgroup"
)

type RelayConfig struct {
	Interval  time.Duration
	BatchSize int
}

type Relay struct {
	repository outboxout.Repository
	publisher  outboxout.Publisher
	logger     *slog.Logger
	interval   time.Duration
	batchSize  int
	now        func() time.Time
}

func NewRelay(repository outboxout.Repository, publisher outboxout.Publisher, cfg RelayConfig, logger *slog.Logger) (*Relay, error) {
	if repository == nil {
		return nil, errors.New("outbox repository is required")
	}
	if publisher == nil {
		return nil, errors.New("outbox publisher is required")
	}
	if cfg.Interval <= 0 {
		return nil, errors.New("outbox interval must be positive")
	}
	if cfg.BatchSize <= 0 {
		return nil, errors.New("outbox batch size must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Relay{
		repository: repository,
		publisher:  publisher,
		logger:     logger.With("component", "outbox_relay"),
		interval:   cfg.Interval,
		batchSize:  cfg.BatchSize,
		now:        time.Now,
	}, nil
}

func (r *Relay) Run(ctx context.Context) error {
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

func (r *Relay) ProcessOnce(ctx context.Context) (int, error) {
	return r.processOnce(ctx)
}

func (r *Relay) processAndLog(ctx context.Context) error {
	startedAt := time.Now()
	processed, err := r.processOnce(ctx)
	duration := time.Since(startedAt)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		r.logger.Error("outbox relay poll failed",
			"batch_size", r.batchSize,
			"processed", processed,
			"duration", duration.String(),
			"error", err,
		)
		return nil
	}

	r.logger.Info("outbox relay poll completed",
		"batch_size", r.batchSize,
		"processed", processed,
		"duration", duration.String(),
	)

	return nil
}

func (r *Relay) processOnce(ctx context.Context) (int, error) {
	return r.repository.ClaimUnpublished(ctx, r.batchSize, func(txCtx context.Context, event domain.Event) error {
		if err := r.publisher.Publish(txCtx, event); err != nil {
			metrics.ObserveKafkaPublishFailure()
			r.logger.Error("outbox event publish failed",
				"event_id", event.ID.String(),
				"event_type", event.EventType,
				"attempt", event.Attempts+1,
				"published", false,
				"error", err,
			)
			if incrementErr := r.repository.IncrementAttempts(txCtx, event.ID); incrementErr != nil {
				return incrementErr
			}

			return nil
		}

		if err := r.repository.MarkPublished(txCtx, event.ID, r.now().UTC()); err != nil {
			return err
		}
		metrics.ObserveKafkaPublished()
		r.logger.Info("outbox event published",
			"event_id", event.ID.String(),
			"event_type", event.EventType,
			"attempt", event.Attempts+1,
			"published", true,
		)

		return nil
	})
}

func RunAll(ctx context.Context, runners ...func(context.Context) error) error {
	group, groupCtx := errgroup.WithContext(ctx)
	for _, run := range runners {
		run := run
		group.Go(func() error {
			return run(groupCtx)
		})
	}

	return group.Wait()
}
