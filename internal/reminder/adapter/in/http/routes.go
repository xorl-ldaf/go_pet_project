package httpadapter

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/tasks/{taskId}/reminders", authenticate(http.HandlerFunc(handler.ListReminders)))
	mux.Handle("POST /api/v1/tasks/{taskId}/reminders", authenticate(http.HandlerFunc(handler.CreateReminder)))
	mux.Handle("PATCH /api/v1/tasks/{taskId}/reminders/{reminderId}", authenticate(http.HandlerFunc(handler.UpdateReminder)))
	mux.Handle("DELETE /api/v1/tasks/{taskId}/reminders/{reminderId}", authenticate(http.HandlerFunc(handler.DeleteReminder)))
}
