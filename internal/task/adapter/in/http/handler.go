package httpadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	"go_pet_project/internal/platform/httpx"
	reminderdomain "go_pet_project/internal/reminder/domain"
	"go_pet_project/internal/task/application"
	"go_pet_project/internal/task/application/command"
	taskin "go_pet_project/internal/task/application/port/in"
	taskout "go_pet_project/internal/task/application/port/out"
	taskquery "go_pet_project/internal/task/application/query"
	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

const maxRequestBodyBytes = 1 << 20

type Handler struct {
	tasks  taskin.TaskService
	logger *slog.Logger
	now    func() time.Time
}

func NewHandler(tasks taskin.TaskService, logger *slog.Logger) *Handler {
	return &Handler{
		tasks:  tasks,
		logger: logger,
		now:    time.Now,
	}
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var request CreateTaskRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if strings.TrimSpace(request.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing required field")
		return
	}

	assigneeID := uuid.Nil
	if request.AssigneeID != nil {
		assigneeID = *request.AssigneeID
	}

	task, err := h.tasks.CreateTask(r.Context(), command.CreateTaskCommand{
		ActorID:     actorID,
		AssigneeID:  assigneeID,
		Title:       request.Title,
		Description: request.Description,
		DeadlineAt:  request.DeadlineAt,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, newTaskResponse(task, h.now().UTC()))
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}

	task, err := h.tasks.GetTask(r.Context(), taskquery.GetTaskQuery{
		ActorID: actorID,
		TaskID:  taskID,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newTaskResponse(task, h.now().UTC()))
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	query, err := parseListTasksQuery(r, actorID, h.now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid query parameter")
		return
	}

	tasks, err := h.tasks.ListTasks(r.Context(), query)
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newTaskListResponse(tasks, h.now().UTC()))
}

func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}

	var request UpdateTaskRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if request.IsEmpty() {
		writeError(w, http.StatusBadRequest, "invalid_request", "empty patch")
		return
	}

	var deadlineAt *command.DeadlineUpdate
	if request.DeadlineAt.Set {
		deadlineAt = &command.DeadlineUpdate{Value: request.DeadlineAt.Value}
	}

	task, err := h.tasks.UpdateTask(r.Context(), command.UpdateTaskCommand{
		ActorID:     actorID,
		TaskID:      taskID,
		AssigneeID:  request.AssigneeID,
		Title:       request.Title,
		Description: request.Description,
		DeadlineAt:  deadlineAt,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newTaskResponse(task, h.now().UTC()))
}

func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}

	task, err := h.tasks.ChangeStatus(r.Context(), command.ChangeStatusCommand{
		ActorID: actorID,
		TaskID:  taskID,
		Status:  domain.StatusDone,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newTaskResponse(task, h.now().UTC()))
}

func (h *Handler) ArchiveTask(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}

	task, err := h.tasks.ArchiveTask(r.Context(), command.ArchiveTaskCommand{
		ActorID: actorID,
		TaskID:  taskID,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newTaskResponse(task, h.now().UTC()))
}

func (h *Handler) RestoreTask(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authhttp.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	taskID, ok := parsePathUUID(w, r, "id")
	if !ok {
		return
	}

	task, err := h.tasks.RestoreTask(r.Context(), command.RestoreTaskCommand{
		ActorID: actorID,
		TaskID:  taskID,
	})
	if err != nil {
		h.writeMappedError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, newTaskResponse(task, h.now().UTC()))
}

func parseListTasksQuery(r *http.Request, actorID uuid.UUID, now time.Time) (taskquery.ListTasksQuery, error) {
	values := r.URL.Query()
	query := taskquery.ListTasksQuery{
		ActorID:  actorID,
		Scope:    taskquery.ListScopeVisible,
		Archived: taskout.ArchivedActiveOnly,
		Sort:     taskout.TaskSortCreatedAtDesc,
	}

	if value := values.Get("view"); value != "" {
		switch value {
		case "all":
			query.Scope = taskquery.ListScopeVisible
		case "assigned":
			query.Scope = taskquery.ListScopeAssignedToMe
		case "created":
			query.Scope = taskquery.ListScopeCreatedByMe
		default:
			return taskquery.ListTasksQuery{}, fmt.Errorf("invalid view")
		}
	}

	if value := values.Get("status"); value != "" {
		status, err := domain.ParseStatus(value)
		if err != nil {
			return taskquery.ListTasksQuery{}, err
		}
		query.Status = &status
	}

	if value := values.Get("archived"); value != "" {
		switch value {
		case "all":
			query.Archived = taskout.ArchivedAll
		default:
			archived, err := strconv.ParseBool(value)
			if err != nil {
				return taskquery.ListTasksQuery{}, err
			}
			if archived {
				query.Archived = taskout.ArchivedOnly
			} else {
				query.Archived = taskout.ArchivedActiveOnly
			}
		}
	}

	if value := values.Get("overdue"); value != "" {
		overdue, err := strconv.ParseBool(value)
		if err != nil {
			return taskquery.ListTasksQuery{}, err
		}
		query.Overdue = &overdue
		query.Now = &now
	}

	if value := values.Get("deadline_from"); value != "" {
		deadlineFrom, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return taskquery.ListTasksQuery{}, err
		}
		query.DeadlineFrom = &deadlineFrom
	}

	if value := values.Get("deadline_to"); value != "" {
		deadlineTo, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return taskquery.ListTasksQuery{}, err
		}
		query.DeadlineTo = &deadlineTo
	}

	query.Search = values.Get("search")

	if value := values.Get("sort"); value != "" {
		switch value {
		case "created_desc":
			query.Sort = taskout.TaskSortCreatedAtDesc
		case "created_asc":
			query.Sort = taskout.TaskSortCreatedAtAsc
		case "deadline_asc":
			query.Sort = taskout.TaskSortDeadlineAtAsc
		case "deadline_desc":
			query.Sort = taskout.TaskSortDeadlineAtDesc
		default:
			return taskquery.ListTasksQuery{}, fmt.Errorf("invalid sort")
		}
	}

	return query, nil
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
	case errors.Is(err, application.ErrTaskAccessDenied),
		errors.Is(err, application.ErrAssignmentDenied):
		writeError(w, http.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, domain.ErrTaskNotFound):
		writeError(w, http.StatusNotFound, "task_not_found", "task not found")
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		writeError(w, http.StatusConflict, "invalid_status_transition", "invalid status transition")
	case errors.Is(err, reminderdomain.ErrTaskDeadlineRequired):
		writeError(w, http.StatusConflict, "invalid_task_deadline", "task deadline is required by pending reminders")
	case errors.Is(err, domain.ErrInvalidTaskID),
		errors.Is(err, domain.ErrInvalidSeriesID),
		errors.Is(err, domain.ErrInvalidCreatorID),
		errors.Is(err, domain.ErrInvalidAssigneeID),
		errors.Is(err, domain.ErrInvalidTitle),
		errors.Is(err, domain.ErrInvalidStatus),
		errors.Is(err, domain.ErrInvalidTimestamp),
		errors.Is(err, taskout.ErrInvalidTaskFilter),
		errors.Is(err, application.ErrInvalidTaskListScope):
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request")
	default:
		if h.logger != nil {
			h.logger.Error("task HTTP request failed", "error", err)
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
