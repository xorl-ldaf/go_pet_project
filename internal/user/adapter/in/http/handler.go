package httpadapter

import (
	"errors"
	"log/slog"
	"net/http"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	"go_pet_project/internal/platform/httpx"
	"go_pet_project/internal/user/application"
	userin "go_pet_project/internal/user/application/port/in"
	"go_pet_project/internal/user/application/query"
	"go_pet_project/internal/user/domain"
)

type Handler struct {
	users  userin.UserService
	logger *slog.Logger
}

func NewHandler(users userin.UserService, logger *slog.Logger) *Handler {
	return &Handler{
		users:  users,
		logger: logger,
	}
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	result, err := h.users.GetMe(r.Context(), query.GetMeQuery{UserID: userID})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newGetMeResponse(result))
}

func (h *Handler) ListAssignableUsers(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	result, err := h.users.ListAssignableUsers(r.Context(), query.ListAssignableUsersQuery{ActorID: userID})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newAssignableUsersResponse(result))
}

func (h *Handler) writeMappedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidActor):
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
	case errors.Is(err, domain.ErrUserNotFound):
		httpx.WriteError(w, http.StatusNotFound, "user_not_found", "user not found")
	default:
		if h.logger != nil {
			h.logger.Error("user HTTP request failed", "error", err)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_server_error", "internal server error")
	}
}
