package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	notificationout "go_pet_project/internal/notification/application/port/out"
	"go_pet_project/internal/notification/domain"
	"go_pet_project/internal/platform/metrics"
)

const maxDeliveryErrorLength = 500

type DeliveryRunner struct {
	notifications notificationout.NotificationRepository
	deliveries    notificationout.DeliveryRepository
	telegramLinks notificationout.TelegramLinkRepository
	transactions  notificationout.TransactionRunner
	sender        notificationout.MessageSender
	retryDelays   []time.Duration
	interval      time.Duration
	batchSize     int
	logger        *slog.Logger
	now           func() time.Time
}

func NewDeliveryRunner(
	notifications notificationout.NotificationRepository,
	deliveries notificationout.DeliveryRepository,
	telegramLinks notificationout.TelegramLinkRepository,
	transactions notificationout.TransactionRunner,
	sender notificationout.MessageSender,
	retryDelays []time.Duration,
	interval time.Duration,
	batchSize int,
	logger *slog.Logger,
) (*DeliveryRunner, error) {
	if notifications == nil {
		return nil, errors.New("notification repository is required")
	}
	if deliveries == nil {
		return nil, errors.New("delivery repository is required")
	}
	if telegramLinks == nil {
		return nil, errors.New("telegram link repository is required")
	}
	if transactions == nil {
		return nil, errors.New("transaction runner is required")
	}
	if sender == nil {
		return nil, errors.New("message sender is required")
	}
	if len(retryDelays) == 0 {
		return nil, errors.New("retry delays are required")
	}
	if interval <= 0 {
		return nil, errors.New("delivery interval must be positive")
	}
	if batchSize <= 0 {
		return nil, errors.New("delivery batch size must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &DeliveryRunner{
		notifications: notifications,
		deliveries:    deliveries,
		telegramLinks: telegramLinks,
		transactions:  transactions,
		sender:        sender,
		retryDelays:   append([]time.Duration(nil), retryDelays...),
		interval:      interval,
		batchSize:     batchSize,
		logger:        logger.With("component", "telegram_delivery_runner"),
		now:           time.Now,
	}, nil
}

func (r *DeliveryRunner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		if err := r.ProcessDue(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			r.logger.Error("telegram delivery processing failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *DeliveryRunner) ProcessDue(ctx context.Context) error {
	return r.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		deliveries, err := r.deliveries.ClaimDue(txCtx, r.now().UTC(), r.batchSize)
		if err != nil {
			return err
		}

		for _, delivery := range deliveries {
			if err := r.processDelivery(txCtx, delivery); err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *DeliveryRunner) processDelivery(ctx context.Context, delivery domain.NotificationDelivery) error {
	notification, err := r.notifications.FindByID(ctx, delivery.NotificationID)
	if err != nil {
		return fmt.Errorf("load notification for delivery: %w", err)
	}

	link, err := r.telegramLinks.FindByUserID(ctx, notification.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrTelegramLinkNotFound) {
			return r.markFailed(ctx, delivery, "telegram link unavailable")
		}
		return fmt.Errorf("load telegram link for delivery: %w", err)
	}
	if !link.Enabled {
		return r.markFailed(ctx, delivery, "telegram link disabled")
	}

	attempts := delivery.Attempts + 1
	if err := r.sender.Send(ctx, notificationout.Message{
		ChatID: link.ChatID,
		Title:  notification.Title,
		Body:   notification.Body,
	}); err != nil {
		return r.handleSendFailure(ctx, delivery, attempts, err)
	}

	metrics.ObserveTelegramDeliverySuccess()
	return r.deliveries.MarkSent(ctx, delivery.ID, attempts, r.now().UTC())
}

func (r *DeliveryRunner) handleSendFailure(ctx context.Context, delivery domain.NotificationDelivery, attempts int, sendErr error) error {
	lastError := sanitizeDeliveryError(sendErr)
	nextDelayIndex := attempts - 1
	if nextDelayIndex < len(r.retryDelays) {
		metrics.ObserveTelegramDeliveryRetry()
		return r.deliveries.ScheduleRetry(ctx, delivery.ID, attempts, r.now().UTC().Add(r.retryDelays[nextDelayIndex]), lastError)
	}

	metrics.ObserveTelegramDeliveryFailure()
	return r.deliveries.MarkFailed(ctx, delivery.ID, attempts, lastError)
}

func (r *DeliveryRunner) markFailed(ctx context.Context, delivery domain.NotificationDelivery, reason string) error {
	metrics.ObserveTelegramDeliveryFailure()
	return r.deliveries.MarkFailed(ctx, delivery.ID, delivery.Attempts, sanitizeDeliveryError(errors.New(reason)))
}

func sanitizeDeliveryError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\r", " ")
	if len(message) > maxDeliveryErrorLength {
		message = message[:maxDeliveryErrorLength]
	}

	return message
}
