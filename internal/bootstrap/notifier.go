package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	notificationkafka "go_pet_project/internal/notification/adapter/in/kafka"
	notificationtelegramin "go_pet_project/internal/notification/adapter/in/telegram"
	notificationpostgres "go_pet_project/internal/notification/adapter/out/postgres"
	notificationtelegramout "go_pet_project/internal/notification/adapter/out/telegram"
	notificationservice "go_pet_project/internal/notification/application/service"
	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/logging"
	"go_pet_project/internal/platform/server"

	"golang.org/x/sync/errgroup"
)

type Notifier struct {
	cfg      *config.Config
	logger   *slog.Logger
	db       *database.Postgres
	consumer *notificationkafka.Consumer
	poller   *notificationtelegramin.Poller
	runner   *notificationservice.DeliveryRunner
	metrics  *server.MetricsServer
}

func NewNotifier(ctx context.Context, cfg *config.Config) (*Notifier, error) {
	logger := logging.New()

	db, err := database.Open(ctx, cfg.DB, logger)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	notificationRepository := notificationpostgres.NewNotificationRepository(db.GORM)
	processedEventRepository := notificationpostgres.NewProcessedEventRepository(db.GORM)
	deliveryRepository := notificationpostgres.NewDeliveryRepository(db.GORM)
	telegramLinkRepository := notificationpostgres.NewTelegramLinkRepository(db.GORM)
	telegramLinkTokenRepository := notificationpostgres.NewTelegramLinkTokenRepository(db.GORM)
	transactionRunner := database.NewTransactionRunner(db.GORM)

	notificationService, err := notificationservice.NewNotificationService(
		notificationRepository,
		processedEventRepository,
		transactionRunner,
		cfg.Kafka.NotificationGroup,
		notificationservice.WithTelegramRepositories(deliveryRepository, telegramLinkRepository, telegramLinkTokenRepository),
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create notification service: %w", err)
	}

	consumer, err := notificationkafka.NewConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.NotificationTopic,
		cfg.Kafka.NotificationGroup,
		notificationService,
		logger,
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create notification kafka consumer: %w", err)
	}

	var poller *notificationtelegramin.Poller
	var runner *notificationservice.DeliveryRunner
	if cfg.Telegram.BotToken == "" {
		logger.Info("telegram adapter disabled: TELEGRAM_BOT_TOKEN is not set")
	} else {
		telegramClient, err := notificationtelegramout.NewClient(cfg.Telegram.BotToken)
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("create telegram client: %w", err)
		}
		poller = notificationtelegramin.NewPoller(telegramClient, notificationService, logger)
		runner, err = notificationservice.NewDeliveryRunner(
			notificationRepository,
			deliveryRepository,
			telegramLinkRepository,
			transactionRunner,
			notificationtelegramout.NewSender(telegramClient),
			cfg.Telegram.RetryDelays,
			cfg.Telegram.DeliveryInterval,
			cfg.Telegram.DeliveryBatchSize,
			logger,
		)
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("create telegram delivery runner: %w", err)
		}
	}

	var metricsServer *server.MetricsServer
	if cfg.Metrics.Port > 0 {
		metricsServer = server.NewMetricsServer(cfg.Metrics.Port, logger)
	}

	return &Notifier{
		cfg:      cfg,
		logger:   logger,
		db:       db,
		consumer: consumer,
		poller:   poller,
		runner:   runner,
		metrics:  metricsServer,
	}, nil
}

func (n *Notifier) Run(ctx context.Context) error {
	n.logger.Info("notifier startup",
		"database", n.cfg.DB.RedactedURL(),
		"kafka_topic", n.cfg.Kafka.NotificationTopic,
		"kafka_group", n.cfg.Kafka.NotificationGroup,
		"metrics_port", n.cfg.Metrics.Port,
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return n.consumer.Run(groupCtx)
	})
	if n.poller != nil {
		group.Go(func() error {
			return n.poller.Run(groupCtx)
		})
	}
	if n.runner != nil {
		group.Go(func() error {
			return n.runner.Run(groupCtx)
		})
	}
	if n.metrics != nil {
		group.Go(func() error {
			return n.metrics.Run(groupCtx)
		})
	}

	return group.Wait()
}

func (n *Notifier) Shutdown(ctx context.Context) error {
	n.logger.Info("notifier shutdown started")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	n.consumer.Close()

	done := make(chan error, 1)
	go func() {
		done <- n.db.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("close database: %w", err)
		}
	case <-shutdownCtx.Done():
		return fmt.Errorf("notifier shutdown timed out: %w", shutdownCtx.Err())
	}

	n.logger.Info("notifier shutdown completed")

	return nil
}
