package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	notificationin "go_pet_project/internal/notification/application/port/in"
	"go_pet_project/internal/platform/metrics"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Consumer struct {
	client  kafkaClient
	service notificationin.NotificationService
	logger  *slog.Logger
}

type kafkaClient interface {
	PollFetches(ctx context.Context) kgo.Fetches
	CommitRecords(ctx context.Context, rs ...*kgo.Record) error
	Close()
}

func NewConsumer(brokers []string, topic string, group string, service notificationin.NotificationService, logger *slog.Logger) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}
	if topic == "" {
		return nil, fmt.Errorf("kafka topic is required")
	}
	if group == "" {
		return nil, fmt.Errorf("kafka consumer group is required")
	}
	if service == nil {
		return nil, fmt.Errorf("notification service is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer: %w", err)
	}

	return &Consumer{
		client:  client,
		service: service,
		logger:  logger.With("component", "notification_consumer"),
	}, nil
}

func (c *Consumer) Run(ctx context.Context) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			c.logger.Error("notification consumer poll failed", "error", err)
			continue
		}

		for _, record := range fetches.Records() {
			if err := c.handleRecord(ctx, record); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				metrics.ObserveKafkaConsumerFailure()
				c.logger.Error("notification consumer record failed",
					"topic", record.Topic,
					"partition", record.Partition,
					"offset", record.Offset,
					"error", err,
				)
				return err
			}
		}
	}
}

func (c *Consumer) Close() {
	c.client.Close()
}

func (c *Consumer) handleRecord(ctx context.Context, record *kgo.Record) error {
	cmd, err := decodeNotificationRequested(record.Value)
	if err != nil {
		c.logger.Error("invalid notification event",
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"error", err,
		)
		return c.client.CommitRecords(ctx, record)
	}

	if err := c.service.HandleNotificationRequested(ctx, cmd); err != nil {
		return fmt.Errorf("handle notification requested: %w", err)
	}
	if err := c.client.CommitRecords(ctx, record); err != nil {
		return fmt.Errorf("commit notification kafka offset: %w", err)
	}
	metrics.ObserveKafkaConsumerProcessed()

	return nil
}
