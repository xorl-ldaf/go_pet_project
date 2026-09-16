package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	outboxkafka "go_pet_project/internal/outbox/adapter/out/kafka"
	outboxpostgres "go_pet_project/internal/outbox/adapter/out/postgres"
	outboxservice "go_pet_project/internal/outbox/application/service"
	recurrencescheduler "go_pet_project/internal/recurrence/adapter/in/scheduler"
	recurrencepostgres "go_pet_project/internal/recurrence/adapter/out/postgres"
	recurrenceservice "go_pet_project/internal/recurrence/application/service"
	reminderscheduler "go_pet_project/internal/reminder/adapter/in/scheduler"
	reminderpostgres "go_pet_project/internal/reminder/adapter/out/postgres"
	taskpostgres "go_pet_project/internal/task/adapter/out/postgres"

	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/logging"
	"go_pet_project/internal/platform/server"

	"golang.org/x/sync/errgroup"
)

type Scheduler struct {
	cfg              *config.Config
	logger           *slog.Logger
	db               *database.Postgres
	reminderRunner   *reminderscheduler.Runner
	recurrenceRunner *recurrencescheduler.Runner
	outboxRelay      *outboxservice.Relay
	kafkaPublisher   *outboxkafka.Publisher
	metricsServer    *server.MetricsServer
}

func NewScheduler(ctx context.Context, cfg *config.Config) (*Scheduler, error) {
	logger := logging.New()

	db, err := database.Open(ctx, cfg.DB, logger)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	reminderRepository := reminderpostgres.NewRepository(db.GORM)
	taskRepository := taskpostgres.NewRepository(db.GORM)
	outboxRepository := outboxpostgres.NewRepository(db.GORM)
	recurrenceRepository := recurrencepostgres.NewRepository(db.GORM)
	transactionRunner := database.NewTransactionRunner(db.GORM)

	notificationProcessor, err := reminderscheduler.NewNotificationProcessor(taskRepository, reminderRepository, outboxRepository)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create reminder notification processor: %w", err)
	}
	reminderRunner, err := reminderscheduler.NewRunner(
		reminderRepository,
		notificationProcessor,
		reminderscheduler.Config{
			Interval:  cfg.Scheduler.Interval,
			BatchSize: cfg.Scheduler.BatchSize,
			Workers:   cfg.Scheduler.Workers,
		},
		logger,
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create reminder scheduler runner: %w", err)
	}

	recurrenceProcessor, err := recurrenceservice.NewProcessor(recurrenceRepository, taskRepository, reminderRepository, transactionRunner)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create recurrence processor: %w", err)
	}
	recurrenceRunner, err := recurrencescheduler.NewRunner(
		recurrenceProcessor,
		recurrencescheduler.Config{
			Interval:  cfg.Recurrence.Interval,
			BatchSize: cfg.Recurrence.BatchSize,
		},
		logger,
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create recurrence scheduler runner: %w", err)
	}

	kafkaPublisher, err := outboxkafka.NewPublisher(cfg.Kafka.Brokers, cfg.Kafka.NotificationTopic)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create kafka publisher: %w", err)
	}

	outboxRelay, err := outboxservice.NewRelay(
		outboxRepository,
		kafkaPublisher,
		outboxservice.RelayConfig{
			Interval:  cfg.Outbox.Interval,
			BatchSize: cfg.Outbox.BatchSize,
		},
		logger,
	)
	if err != nil {
		kafkaPublisher.Close()
		_ = db.Close()
		return nil, fmt.Errorf("create outbox relay: %w", err)
	}

	var metricsServer *server.MetricsServer
	if cfg.Metrics.Port > 0 {
		metricsServer = server.NewMetricsServer(cfg.Metrics.Port, logger)
	}

	return &Scheduler{
		cfg:              cfg,
		logger:           logger,
		db:               db,
		reminderRunner:   reminderRunner,
		recurrenceRunner: recurrenceRunner,
		outboxRelay:      outboxRelay,
		kafkaPublisher:   kafkaPublisher,
		metricsServer:    metricsServer,
	}, nil
}

func (s *Scheduler) Run(ctx context.Context) error {
	s.logger.Info("scheduler startup",
		"database", s.cfg.DB.RedactedURL(),
		"interval", s.cfg.Scheduler.Interval.String(),
		"batch_size", s.cfg.Scheduler.BatchSize,
		"workers", s.cfg.Scheduler.Workers,
		"recurrence_interval", s.cfg.Recurrence.Interval.String(),
		"recurrence_batch_size", s.cfg.Recurrence.BatchSize,
		"outbox_interval", s.cfg.Outbox.Interval.String(),
		"outbox_batch_size", s.cfg.Outbox.BatchSize,
		"metrics_port", s.cfg.Metrics.Port,
		"kafka_topic", s.cfg.Kafka.NotificationTopic,
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return s.reminderRunner.Run(groupCtx)
	})
	group.Go(func() error {
		return s.outboxRelay.Run(groupCtx)
	})
	group.Go(func() error {
		return s.recurrenceRunner.Run(groupCtx)
	})
	if s.metricsServer != nil {
		group.Go(func() error {
			return s.metricsServer.Run(groupCtx)
		})
	}

	return group.Wait()
}

func (s *Scheduler) Shutdown(ctx context.Context) error {
	s.logger.Info("scheduler shutdown started")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	s.kafkaPublisher.Close()

	done := make(chan error, 1)
	go func() {
		done <- s.db.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("close database: %w", err)
		}
	case <-shutdownCtx.Done():
		return fmt.Errorf("scheduler shutdown timed out: %w", shutdownCtx.Err())
	}

	s.logger.Info("scheduler shutdown completed")

	return nil
}
