package httpadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"go_pet_project/internal/auth/application"
	authin "go_pet_project/internal/auth/application/port/in"
	authdomain "go_pet_project/internal/auth/domain"
	"go_pet_project/internal/platform/httpx"
)

const maxRequestBodyBytes = 1 << 20

type Handler struct {
	auth   authin.AuthService
	logger *slog.Logger
}

func NewHandler(auth authin.AuthService, logger *slog.Logger) *Handler {
	return &Handler{
		auth:   auth,
		logger: logger,
	}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request RegisterRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if request.Email == "" || request.Username == "" || request.Password == "" || request.Timezone == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing required field")
		return
	}

	result, err := h.auth.Register(r.Context(), request.Command())
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, newRegisterResponse(result))
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request LoginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if request.Email == "" || request.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing required field")
		return
	}

	result, err := h.auth.Login(r.Context(), request.Command())
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newLoginResponse(result))
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var request RefreshRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if request.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing required field")
		return
	}

	result, err := h.auth.Refresh(r.Context(), request.Command())
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newRefreshResponse(result))
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var request LogoutRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if request.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing required field")
		return
	}

	if _, err := h.auth.Logout(r.Context(), request.Command()); err != nil {
		h.writeMappedError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return fmt.Errorf("content type must be application/json")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	return fmt.Errorf("request body must contain a single JSON object")
}

func (h *Handler) writeMappedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidEmail),
		errors.Is(err, application.ErrInvalidUsername),
		errors.Is(err, application.ErrInvalidPassword),
		errors.Is(err, application.ErrInvalidTimezone),
		errors.Is(err, application.ErrInvalidRefreshToken):
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, application.ErrEmailAlreadyExists):
		writeError(w, http.StatusConflict, "email_already_exists", "email already exists")
	case errors.Is(err, application.ErrUsernameAlreadyExists):
		writeError(w, http.StatusConflict, "username_already_exists", "username already exists")
	case errors.Is(err, application.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
	case errors.Is(err, authdomain.ErrRefreshTokenNotFound),
		errors.Is(err, authdomain.ErrRefreshTokenExpired),
		errors.Is(err, authdomain.ErrRefreshTokenRevoked):
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "invalid refresh token")
	default:
		if h.logger != nil {
			h.logger.Error("auth HTTP request failed", "error", err)
		}
		writeError(w, http.StatusInternalServerError, "internal_server_error", "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	httpx.WriteError(w, status, code, message)
}

func isJSONContentType(contentType string) bool {
	if contentType == "" {
		return true
	}

	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "application/json"
}
