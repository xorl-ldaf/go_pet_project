package httpadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	"go_pet_project/internal/platform/httpx"
	"go_pet_project/internal/reminder/application"
	"go_pet_project/internal/reminder/application/command"
	reminderin "go_pet_project/internal/reminder/application/port/in"
	reminderquery "go_pet_project/internal/reminder/application/query"
	"go_pet_project/internal/reminder/domain"
	taskdomain "go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

const maxRequestBodyBytes = 1 << 20

type Handler struct {
	reminders reminderin.ReminderService
	logger    *slog.Logger
}

func NewHandler(reminders reminderin.ReminderService, logger *slog.Logger) *Handler {
	return &Handler{reminders: reminders, logger: logger}
}

func (h *Handler) ListReminders(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "taskId")
	if !ok {
		return
	}

	reminders, err := h.reminders.ListReminders(r.Context(), reminderquery.ListRemindersQuery{
		ActorID: actorID,
		TaskID:  taskID,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newReminderListResponse(reminders))
}

func (h *Handler) CreateReminder(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "taskId")
	if !ok {
		return
	}

	var request CreateReminderRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	kind, err := domain.ParseKind(request.Kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid reminder kind")
		return
	}

	reminder, err := h.reminders.CreateReminder(r.Context(), command.CreateReminderCommand{
		ActorID:       actorID,
		TaskID:        taskID,
		Kind:          kind,
		OffsetSeconds: request.OffsetSeconds,
		TriggerAt:     request.TriggerAt,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, newReminderResponse(reminder))
}

func (h *Handler) UpdateReminder(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "taskId")
	if !ok {
		return
	}
	reminderID, ok := parsePathUUID(w, r, "reminderId")
	if !ok {
		return
	}

	var request UpdateReminderRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if request.IsEmpty() {
		writeError(w, http.StatusBadRequest, "invalid_request", "empty patch")
		return
	}

	reminder, err := h.reminders.UpdateReminder(r.Context(), command.UpdateReminderCommand{
		ActorID:       actorID,
		TaskID:        taskID,
		ReminderID:    reminderID,
		OffsetSeconds: request.OffsetSeconds,
		TriggerAt:     request.TriggerAt,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newReminderResponse(reminder))
}

func (h *Handler) DeleteReminder(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "taskId")
	if !ok {
		return
	}
	reminderID, ok := parsePathUUID(w, r, "reminderId")
	if !ok {
		return
	}

	if err := h.reminders.DeleteReminder(r.Context(), command.DeleteReminderCommand{
		ActorID:    actorID,
		TaskID:     taskID,
		ReminderID: reminderID,
	}); err != nil {
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
	case errors.Is(err, application.ErrInvalidActor):
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
	case errors.Is(err, application.ErrReminderAccessDenied):
		writeError(w, http.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, taskdomain.ErrTaskNotFound):
		writeError(w, http.StatusNotFound, "task_not_found", "task not found")
	case errors.Is(err, domain.ErrReminderNotFound):
		writeError(w, http.StatusNotFound, "reminder_not_found", "reminder not found")
	case errors.Is(err, domain.ErrInvalidReminderLifecycle),
		errors.Is(err, domain.ErrTaskAlreadyCompleted),
		errors.Is(err, domain.ErrTaskDeadlineRequired):
		writeError(w, http.StatusConflict, "invalid_reminder_state", "invalid reminder state")
	case errors.Is(err, domain.ErrInvalidReminderID),
		errors.Is(err, domain.ErrInvalidTaskID),
		errors.Is(err, domain.ErrInvalidKind),
		errors.Is(err, domain.ErrInvalidOffsetSeconds),
		errors.Is(err, domain.ErrInvalidTriggerAt),
		errors.Is(err, domain.ErrInvalidState),
		errors.Is(err, domain.ErrInvalidCreatedAt),
		errors.Is(err, domain.ErrInvalidSentAt),
		errors.Is(err, domain.ErrInvalidReminderFields):
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request")
	default:
		if h.logger != nil {
			h.logger.Error("reminder HTTP request failed", "error", err)
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
