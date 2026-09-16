package httpadapter

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	"go_pet_project/internal/notification/application"
	"go_pet_project/internal/notification/application/command"
	notificationin "go_pet_project/internal/notification/application/port/in"
	"go_pet_project/internal/notification/application/query"
	"go_pet_project/internal/notification/domain"
	"go_pet_project/internal/platform/httpx"

	"github.com/google/uuid"
)

type Handler struct {
	notifications notificationin.NotificationService
	logger        *slog.Logger
	botUsername   string
}

func NewHandler(notifications notificationin.NotificationService, logger *slog.Logger, botUsername ...string) *Handler {
	username := ""
	if len(botUsername) > 0 {
		username = botUsername[0]
	}

	return &Handler{notifications: notifications, logger: logger, botUsername: username}
}

func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	notifications, err := h.notifications.ListNotifications(r.Context(), query.ListNotificationsQuery{ActorID: actorID})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newNotificationListResponse(notifications))
}

func (h *Handler) CountUnread(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	count, err := h.notifications.CountUnread(r.Context(), query.CountUnreadQuery{ActorID: actorID})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, UnreadCountResponse{Count: count})
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	notificationID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}

	if err := h.notifications.MarkRead(r.Context(), command.MarkReadCommand{
		ActorID:        actorID,
		NotificationID: notificationID,
	}); err != nil {
		h.writeMappedError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	if err := h.notifications.MarkAllRead(r.Context(), command.MarkAllReadCommand{ActorID: actorID}); err != nil {
		h.writeMappedError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateTelegramLink(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	result, err := h.notifications.CreateTelegramLinkToken(r.Context(), command.CreateTelegramLinkCommand{ActorID: actorID})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newTelegramLinkTokenResponse(result, h.botUsername))
}

func (h *Handler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	settings, err := h.notifications.GetNotificationSettings(r.Context(), query.GetNotificationSettingsQuery{ActorID: actorID})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newNotificationSettingsResponse(settings))
}

func (h *Handler) PatchNotificationSettings(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var request PatchNotificationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if request.Telegram == nil || request.Telegram.Enabled == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "telegram.enabled is required")
		return
	}

	if err := h.notifications.SetTelegramEnabled(r.Context(), command.SetTelegramEnabledCommand{
		ActorID: actorID,
		Enabled: *request.Telegram.Enabled,
	}); err != nil {
		h.writeMappedError(w, err)
		return
	}

	settings, err := h.notifications.GetNotificationSettings(r.Context(), query.GetNotificationSettingsQuery{ActorID: actorID})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newNotificationSettingsResponse(settings))
}

func (h *Handler) DeleteTelegramLink(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	if err := h.notifications.DeleteTelegramLink(r.Context(), command.DeleteTelegramLinkCommand{ActorID: actorID}); err != nil {
		h.writeMappedError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parsePathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid path parameter")
		return uuid.Nil, false
	}

	return id, true
}

func (h *Handler) writeMappedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidActor):
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
	case errors.Is(err, application.ErrInvalidTelegramToken),
		errors.Is(err, domain.ErrTelegramLinkTokenInvalidOrExpired):
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid telegram link token")
	case errors.Is(err, domain.ErrNotificationNotFound):
		writeError(w, http.StatusNotFound, "notification_not_found", "notification not found")
	case errors.Is(err, domain.ErrTelegramLinkNotFound):
		writeError(w, http.StatusNotFound, "telegram_link_not_found", "telegram link not found")
	case errors.Is(err, domain.ErrInvalidNotificationID),
		errors.Is(err, domain.ErrInvalidUserID),
		errors.Is(err, domain.ErrInvalidTaskID),
		errors.Is(err, domain.ErrInvalidReminderID),
		errors.Is(err, domain.ErrInvalidType),
		errors.Is(err, domain.ErrInvalidTitle),
		errors.Is(err, domain.ErrInvalidBody),
		errors.Is(err, domain.ErrInvalidCreatedAt),
		errors.Is(err, domain.ErrInvalidReadAt),
		errors.Is(err, domain.ErrInvalidTelegramChatID),
		errors.Is(err, domain.ErrInvalidTelegramLinkedAt),
		errors.Is(err, domain.ErrInvalidTelegramLinkTokenID),
		errors.Is(err, domain.ErrInvalidTelegramLinkToken),
		errors.Is(err, domain.ErrInvalidTelegramLinkTokenExpiry),
		errors.Is(err, domain.ErrInvalidTelegramLinkTokenUsedAt):
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request")
	default:
		if h.logger != nil {
			h.logger.Error("notification HTTP request failed", "error", err)
		}
		writeError(w, http.StatusInternalServerError, "internal_server_error", "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	httpx.WriteError(w, status, code, message)
}
