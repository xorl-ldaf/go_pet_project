package httpadapter

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, authenticate func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/tasks", authenticate(http.HandlerFunc(handler.CreateTask)))
	mux.Handle("GET /api/v1/tasks", authenticate(http.HandlerFunc(handler.ListTasks)))
	mux.Handle("GET /api/v1/tasks/{id}", authenticate(http.HandlerFunc(handler.GetTask)))
	mux.Handle("PATCH /api/v1/tasks/{id}", authenticate(http.HandlerFunc(handler.UpdateTask)))
	mux.Handle("POST /api/v1/tasks/{id}/complete", authenticate(http.HandlerFunc(handler.CompleteTask)))
	mux.Handle("POST /api/v1/tasks/{id}/archive", authenticate(http.HandlerFunc(handler.ArchiveTask)))
	mux.Handle("POST /api/v1/tasks/{id}/restore", authenticate(http.HandlerFunc(handler.RestoreTask)))
}
