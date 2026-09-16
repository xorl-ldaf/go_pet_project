package httpadapter

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/notifications", authenticate(http.HandlerFunc(handler.ListNotifications)))
	mux.Handle("GET /api/v1/notifications/unread-count", authenticate(http.HandlerFunc(handler.CountUnread)))
	mux.Handle("POST /api/v1/notifications/{id}/read", authenticate(http.HandlerFunc(handler.MarkRead)))
	mux.Handle("POST /api/v1/notifications/read-all", authenticate(http.HandlerFunc(handler.MarkAllRead)))
	mux.Handle("POST /api/v1/users/me/telegram/link", authenticate(http.HandlerFunc(handler.CreateTelegramLink)))
	mux.Handle("DELETE /api/v1/users/me/telegram/link", authenticate(http.HandlerFunc(handler.DeleteTelegramLink)))
	mux.Handle("GET /api/v1/users/me/notification-settings", authenticate(http.HandlerFunc(handler.GetNotificationSettings)))
	mux.Handle("PATCH /api/v1/users/me/notification-settings", authenticate(http.HandlerFunc(handler.PatchNotificationSettings)))
}
