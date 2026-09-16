package httpadapter

import "net/http"

func RegisterRoutes(mux *http.ServeMux, handler *Handler, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/users/me", authenticate(http.HandlerFunc(handler.GetMe)))
	mux.Handle("GET /api/v1/users/assignable", authenticate(http.HandlerFunc(handler.ListAssignableUsers)))
}
