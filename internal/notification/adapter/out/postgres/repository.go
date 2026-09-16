package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	notificationout "go_pet_project/internal/notification/application/port/out"
	"go_pet_project/internal/notification/domain"
	"go_pet_project/internal/platform/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ notificationout.NotificationRepository = (*NotificationRepository)(nil)
var _ notificationout.ProcessedEventRepository = (*ProcessedEventRepository)(nil)
var _ notificationout.TelegramLinkRepository = (*TelegramLinkRepository)(nil)
var _ notificationout.TelegramLinkTokenRepository = (*TelegramLinkTokenRepository)(nil)
var _ notificationout.DeliveryRepository = (*DeliveryRepository)(nil)

type NotificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(ctx context.Context, notification domain.Notification) (domain.Notification, error) {
	model := toNotificationModel(notification)
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Create(&model).Error; err != nil {
		return domain.Notification{}, fmt.Errorf("create notification: %w", err)
	}

	created, err := toNotificationDomain(model)
	if err != nil {
		return domain.Notification{}, fmt.Errorf("create notification: %w", err)
	}

	return created, nil
}

func (r *NotificationRepository) FindByID(ctx context.Context, id uuid.UUID) (domain.Notification, error) {
	var model notificationModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Notification{}, mapNotificationFindError("find notification by id", err)
	}

	notification, err := toNotificationDomain(model)
	if err != nil {
		return domain.Notification{}, fmt.Errorf("find notification by id: %w", err)
	}

	return notification, nil
}

func (r *NotificationRepository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Notification, error) {
	var models []notificationModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list notifications by user id: %w", err)
	}

	notifications := make([]domain.Notification, 0, len(models))
	for _, model := range models {
		notification, err := toNotificationDomain(model)
		if err != nil {
			return nil, fmt.Errorf("list notifications by user id: %w", err)
		}
		notifications = append(notifications, notification)
	}

	return notifications, nil
}

func (r *NotificationRepository) CountUnreadByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int64
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&notificationModel{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}

	return int(count), nil
}

func (r *NotificationRepository) MarkRead(ctx context.Context, id uuid.UUID, readAt time.Time) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&notificationModel{}).
		Where("id = ?", id).
		Update("read_at", readAt)
	if result.Error != nil {
		return fmt.Errorf("mark notification read: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("mark notification read: %w", domain.ErrNotificationNotFound)
	}

	return nil
}

func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID uuid.UUID, readAt time.Time) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&notificationModel{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", readAt)
	if result.Error != nil {
		return fmt.Errorf("mark all notifications read: %w", result.Error)
	}

	return nil
}

type ProcessedEventRepository struct {
	db *gorm.DB
}

func NewProcessedEventRepository(db *gorm.DB) *ProcessedEventRepository {
	return &ProcessedEventRepository{db: db}
}

func (r *ProcessedEventRepository) TryInsert(ctx context.Context, consumerName string, eventID uuid.UUID, processedAt time.Time) (bool, error) {
	model := processedEventModel{
		ConsumerName: consumerName,
		EventID:      eventID,
		ProcessedAt:  processedAt,
	}
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model)
	if result.Error != nil {
		return false, fmt.Errorf("insert processed event: %w", result.Error)
	}

	return result.RowsAffected == 1, nil
}

func (r *ProcessedEventRepository) Exists(ctx context.Context, consumerName string, eventID uuid.UUID) (bool, error) {
	var exists bool
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Raw(`
			SELECT EXISTS (
				SELECT 1
				FROM processed_events
				WHERE consumer_name = ?
					AND event_id = ?
			)
		`, consumerName, eventID).
		Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("check processed event exists: %w", err)
	}

	return exists, nil
}

func mapNotificationFindError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotificationNotFound)
	}

	return fmt.Errorf("%s: %w", operation, err)
}

type TelegramLinkRepository struct {
	db *gorm.DB
}

func NewTelegramLinkRepository(db *gorm.DB) *TelegramLinkRepository {
	return &TelegramLinkRepository{db: db}
}

func (r *TelegramLinkRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (domain.TelegramLink, error) {
	var model telegramLinkModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).First(&model, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.TelegramLink{}, fmt.Errorf("find telegram link: %w", domain.ErrTelegramLinkNotFound)
		}
		return domain.TelegramLink{}, fmt.Errorf("find telegram link: %w", err)
	}

	link, err := toTelegramLinkDomain(model)
	if err != nil {
		return domain.TelegramLink{}, fmt.Errorf("find telegram link: %w", err)
	}

	return link, nil
}

func (r *TelegramLinkRepository) Upsert(ctx context.Context, link domain.TelegramLink) (domain.TelegramLink, error) {
	model := toTelegramLinkModel(link)
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"chat_id",
				"telegram_username",
				"linked_at",
				"enabled",
			}),
		}).
		Create(&model).Error; err != nil {
		return domain.TelegramLink{}, fmt.Errorf("upsert telegram link: %w", err)
	}

	created, err := toTelegramLinkDomain(model)
	if err != nil {
		return domain.TelegramLink{}, fmt.Errorf("upsert telegram link: %w", err)
	}

	return created, nil
}

func (r *TelegramLinkRepository) SetEnabled(ctx context.Context, userID uuid.UUID, enabled bool) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&telegramLinkModel{}).
		Where("user_id = ?", userID).
		Update("enabled", enabled)
	if result.Error != nil {
		return fmt.Errorf("set telegram link enabled: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("set telegram link enabled: %w", domain.ErrTelegramLinkNotFound)
	}

	return nil
}

func (r *TelegramLinkRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Delete(&telegramLinkModel{}, "user_id = ?", userID).Error; err != nil {
		return fmt.Errorf("delete telegram link: %w", err)
	}

	return nil
}

type TelegramLinkTokenRepository struct {
	db *gorm.DB
}

func NewTelegramLinkTokenRepository(db *gorm.DB) *TelegramLinkTokenRepository {
	return &TelegramLinkTokenRepository{db: db}
}

func (r *TelegramLinkTokenRepository) Create(ctx context.Context, token domain.TelegramLinkToken) (domain.TelegramLinkToken, error) {
	model := toTelegramLinkTokenModel(token)
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Create(&model).Error; err != nil {
		return domain.TelegramLinkToken{}, fmt.Errorf("create telegram link token: %w", err)
	}

	created, err := toTelegramLinkTokenDomain(model)
	if err != nil {
		return domain.TelegramLinkToken{}, fmt.Errorf("create telegram link token: %w", err)
	}

	return created, nil
}

func (r *TelegramLinkTokenRepository) ConsumeByHash(ctx context.Context, tokenHash string, usedAt time.Time) (domain.TelegramLinkToken, error) {
	var model telegramLinkTokenModel
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Raw(`
			UPDATE telegram_link_tokens
			SET used_at = ?
			WHERE token_hash = ?
				AND used_at IS NULL
				AND expires_at > ?
			RETURNING id, user_id, token_hash, expires_at, used_at, created_at
		`, usedAt, tokenHash, usedAt).
		Scan(&model)
	if result.Error != nil {
		return domain.TelegramLinkToken{}, fmt.Errorf("consume telegram link token: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return domain.TelegramLinkToken{}, fmt.Errorf("consume telegram link token: %w", domain.ErrTelegramLinkTokenNotFound)
	}

	token, err := toTelegramLinkTokenDomain(model)
	if err != nil {
		return domain.TelegramLinkToken{}, fmt.Errorf("consume telegram link token: %w", err)
	}

	return token, nil
}

type DeliveryRepository struct {
	db *gorm.DB
}

func NewDeliveryRepository(db *gorm.DB) *DeliveryRepository {
	return &DeliveryRepository{db: db}
}

func (r *DeliveryRepository) Create(ctx context.Context, delivery domain.NotificationDelivery) (domain.NotificationDelivery, error) {
	model := toNotificationDeliveryModel(delivery)
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).Create(&model).Error; err != nil {
		return domain.NotificationDelivery{}, fmt.Errorf("create notification delivery: %w", err)
	}

	created, err := toNotificationDeliveryDomain(model)
	if err != nil {
		return domain.NotificationDelivery{}, fmt.Errorf("create notification delivery: %w", err)
	}

	return created, nil
}

func (r *DeliveryRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.NotificationDelivery, error) {
	var models []notificationDeliveryModel
	if err := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Raw(`
			WITH due AS (
				SELECT id
				FROM notification_deliveries
				WHERE status = ?
					AND next_attempt_at <= ?
				ORDER BY next_attempt_at ASC, id ASC
				LIMIT ?
				FOR UPDATE SKIP LOCKED
			)
			SELECT notification_deliveries.id,
				notification_deliveries.notification_id,
				notification_deliveries.channel,
				notification_deliveries.status,
				notification_deliveries.attempts,
				notification_deliveries.next_attempt_at,
				notification_deliveries.sent_at,
				notification_deliveries.last_error
			FROM notification_deliveries
			INNER JOIN due ON due.id = notification_deliveries.id
			ORDER BY notification_deliveries.next_attempt_at ASC, notification_deliveries.id ASC
		`, string(domain.DeliveryStatusPending), now, limit).
		Scan(&models).Error; err != nil {
		return nil, fmt.Errorf("claim due notification deliveries: %w", err)
	}

	deliveries := make([]domain.NotificationDelivery, 0, len(models))
	for _, model := range models {
		delivery, err := toNotificationDeliveryDomain(model)
		if err != nil {
			return nil, fmt.Errorf("claim due notification deliveries: %w", err)
		}
		deliveries = append(deliveries, delivery)
	}

	return deliveries, nil
}

func (r *DeliveryRepository) MarkSent(ctx context.Context, id uuid.UUID, attempts int, sentAt time.Time) error {
	return r.updateDelivery(ctx, id, map[string]interface{}{
		"status":          string(domain.DeliveryStatusSent),
		"attempts":        attempts,
		"sent_at":         sentAt,
		"next_attempt_at": nil,
		"last_error":      nil,
	})
}

func (r *DeliveryRepository) ScheduleRetry(ctx context.Context, id uuid.UUID, attempts int, nextAttemptAt time.Time, lastError string) error {
	return r.updateDelivery(ctx, id, map[string]interface{}{
		"status":          string(domain.DeliveryStatusPending),
		"attempts":        attempts,
		"next_attempt_at": nextAttemptAt,
		"last_error":      lastError,
	})
}

func (r *DeliveryRepository) MarkFailed(ctx context.Context, id uuid.UUID, attempts int, lastError string) error {
	return r.updateDelivery(ctx, id, map[string]interface{}{
		"status":          string(domain.DeliveryStatusFailed),
		"attempts":        attempts,
		"next_attempt_at": nil,
		"last_error":      lastError,
	})
}

func (r *DeliveryRepository) updateDelivery(ctx context.Context, id uuid.UUID, values map[string]interface{}) error {
	result := database.GORMFromContext(ctx, r.db).WithContext(ctx).
		Model(&notificationDeliveryModel{}).
		Where("id = ?", id).
		Updates(values)
	if result.Error != nil {
		return fmt.Errorf("update notification delivery: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update notification delivery: %w", domain.ErrDeliveryNotFound)
	}

	return nil
}
