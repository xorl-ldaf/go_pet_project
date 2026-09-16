package httpadapter

import (
	"fmt"
	"time"

	"go_pet_project/internal/notification/application/command"
	"go_pet_project/internal/notification/application/query"
	"go_pet_project/internal/notification/domain"
	"go_pet_project/internal/platform/httpx"

	"github.com/google/uuid"
)

type NotificationResponse struct {
	ID         string     `json:"id"`
	TaskID     *string    `json:"task_id"`
	ReminderID *string    `json:"reminder_id"`
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	CreatedAt  time.Time  `json:"created_at"`
	ReadAt     *time.Time `json:"read_at"`
}

type NotificationListResponse struct {
	Items []NotificationResponse `json:"items"`
}

type UnreadCountResponse struct {
	Count int `json:"count"`
}

type TelegramLinkTokenResponse struct {
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"`
	DeepLinkURL *string   `json:"deep_link_url,omitempty"`
}

type NotificationSettingsResponse struct {
	Internal bool                     `json:"internal"`
	Telegram TelegramSettingsResponse `json:"telegram"`
}

type TelegramSettingsResponse struct {
	Linked   bool    `json:"linked"`
	Enabled  bool    `json:"enabled"`
	Username *string `json:"username"`
}

type PatchNotificationSettingsRequest struct {
	Telegram *PatchTelegramSettingsRequest `json:"telegram"`
}

type PatchTelegramSettingsRequest struct {
	Enabled *bool `json:"enabled"`
}

type ErrorResponse = httpx.ErrorResponse
type ErrorBody = httpx.ErrorBody

func newNotificationResponse(notification domain.Notification) NotificationResponse {
	return NotificationResponse{
		ID:         notification.ID.String(),
		TaskID:     uuidStringPtr(notification.TaskID),
		ReminderID: uuidStringPtr(notification.ReminderID),
		Type:       string(notification.Type),
		Title:      notification.Title,
		Body:       notification.Body,
		CreatedAt:  notification.CreatedAt,
		ReadAt:     cloneTimePtr(notification.ReadAt),
	}
}

func newNotificationListResponse(notifications []domain.Notification) NotificationListResponse {
	items := make([]NotificationResponse, 0, len(notifications))
	for _, notification := range notifications {
		items = append(items, newNotificationResponse(notification))
	}

	return NotificationListResponse{Items: items}
}

func newTelegramLinkTokenResponse(result command.TelegramLinkTokenResult, botUsername string) TelegramLinkTokenResponse {
	response := TelegramLinkTokenResponse{
		Token:     result.Token,
		ExpiresAt: result.ExpiresAt,
	}
	if botUsername != "" {
		deepLinkURL := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, result.Token)
		response.DeepLinkURL = &deepLinkURL
	}

	return response
}

func newNotificationSettingsResponse(settings query.NotificationSettings) NotificationSettingsResponse {
	return NotificationSettingsResponse{
		Internal: settings.Internal,
		Telegram: TelegramSettingsResponse{
			Linked:   settings.Telegram.Linked,
			Enabled:  settings.Telegram.Enabled,
			Username: cloneStringPtr(settings.Telegram.Username),
		},
	}
}

func uuidStringPtr(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	copied := value.String()

	return &copied
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}
