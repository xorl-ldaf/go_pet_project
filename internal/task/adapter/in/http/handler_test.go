package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	authout "go_pet_project/internal/auth/application/port/out"
	"go_pet_project/internal/task/application"
	"go_pet_project/internal/task/application/command"
	taskout "go_pet_project/internal/task/application/port/out"
	taskquery "go_pet_project/internal/task/application/query"
	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

var errTaskHTTPFakeInternal = errors.New("database unavailable")

func TestCreateTaskSuccess(t *testing.T) {
	now := testHTTPTime()
	actorID := testHTTPUUID(1)
	deadline := now.Add(24 * time.Hour)
	service := &fakeTaskService{
		createResult: taskDTO(t, testHTTPUUID(10), actorID, actorID, domain.StatusOpen, &deadline, nil),
	}

	recorder := performTaskRequest(t, service, actorID, http.MethodPost, "/api/v1/tasks", `{
		"creator_id": "10000000-0000-4000-8000-000000000099",
		"assignee_id": "10000000-0000-4000-8000-000000000001",
		"title": "Task A",
		"description": "body",
		"deadline_at": "`+deadline.Format(time.RFC3339)+`"
	}`, true)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status with creator_id = %d, want 400", recorder.Code)
	}
	if service.createCalls != 0 {
		t.Fatalf("CreateTask calls = %d, want 0 for unknown creator_id", service.createCalls)
	}

	recorder = performTaskRequest(t, service, actorID, http.MethodPost, "/api/v1/tasks", `{
		"assignee_id": "10000000-0000-4000-8000-000000000001",
		"title": "Task A",
		"description": "body",
		"deadline_at": "`+deadline.Format(time.RFC3339)+`"
	}`, true)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", recorder.Code, recorder.Body.String())
	}
	if service.createCalls != 1 {
		t.Fatalf("CreateTask calls = %d, want 1", service.createCalls)
	}
	cmd := service.lastCreate
	if cmd.ActorID != actorID ||
		cmd.AssigneeID != actorID ||
		cmd.Title != "Task A" ||
		cmd.Description != "body" ||
		cmd.DeadlineAt == nil ||
		!cmd.DeadlineAt.Equal(deadline) {
		t.Fatalf("CreateTask command mismatch: %#v", cmd)
	}

	var response TaskResponse
	decodeTaskResponse(t, recorder, &response)
	if response.CreatorID != actorID.String() || response.AssigneeID != actorID.String() || response.Status != string(domain.StatusOpen) {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestCreateTaskNoAuth(t *testing.T) {
	service := &fakeTaskService{}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks", `{"title":"Task A"}`, false)

	assertTaskErrorResponse(t, recorder, http.StatusUnauthorized, "unauthorized")
	if service.createCalls != 0 {
		t.Fatalf("CreateTask calls = %d, want 0", service.createCalls)
	}
}

func TestCreateTaskInvalidJSON(t *testing.T) {
	service := &fakeTaskService{}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks", `{"title":`, true)

	assertTaskErrorResponse(t, recorder, http.StatusBadRequest, "invalid_request")
	if service.createCalls != 0 {
		t.Fatalf("CreateTask calls = %d, want 0", service.createCalls)
	}
}

func TestCreateTaskForeignAssigneeBlocked(t *testing.T) {
	service := &fakeTaskService{createErr: application.ErrAssignmentDenied}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks", `{
		"assignee_id": "10000000-0000-4000-8000-000000000002",
		"title": "Task A"
	}`, true)

	assertTaskErrorResponse(t, recorder, http.StatusForbidden, "forbidden")
}

func TestGetTaskByIDSuccess(t *testing.T) {
	actorID := testHTTPUUID(1)
	taskID := testHTTPUUID(10)
	service := &fakeTaskService{
		getResult: taskDTO(t, taskID, actorID, actorID, domain.StatusOpen, nil, nil),
	}

	recorder := performTaskRequest(t, service, actorID, http.MethodGet, "/api/v1/tasks/"+taskID.String(), "", true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.getCalls != 1 || service.lastGet.ActorID != actorID || service.lastGet.TaskID != taskID {
		t.Fatalf("GetTask query mismatch: %#v", service.lastGet)
	}
}

func TestGetTaskInvalidUUID(t *testing.T) {
	service := &fakeTaskService{}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodGet, "/api/v1/tasks/not-a-uuid", "", true)

	assertTaskErrorResponse(t, recorder, http.StatusBadRequest, "invalid_request")
	if service.getCalls != 0 {
		t.Fatalf("GetTask calls = %d, want 0", service.getCalls)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	service := &fakeTaskService{getErr: domain.ErrTaskNotFound}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodGet, "/api/v1/tasks/"+testHTTPUUID(10).String(), "", true)

	assertTaskErrorResponse(t, recorder, http.StatusNotFound, "task_not_found")
}

func TestGetTaskForbidden(t *testing.T) {
	service := &fakeTaskService{getErr: application.ErrTaskAccessDenied}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodGet, "/api/v1/tasks/"+testHTTPUUID(10).String(), "", true)

	assertTaskErrorResponse(t, recorder, http.StatusForbidden, "forbidden")
}

func TestListTasksDefault(t *testing.T) {
	actorID := testHTTPUUID(1)
	taskID := testHTTPUUID(10)
	service := &fakeTaskService{
		listResult: []domain.Task{taskDTO(t, taskID, actorID, actorID, domain.StatusOpen, nil, nil)},
	}

	recorder := performTaskRequest(t, service, actorID, http.MethodGet, "/api/v1/tasks", "", true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.listCalls != 1 ||
		service.lastList.ActorID != actorID ||
		service.lastList.Scope != taskquery.ListScopeVisible ||
		service.lastList.Archived != taskout.ArchivedActiveOnly ||
		service.lastList.Sort != taskout.TaskSortCreatedAtDesc {
		t.Fatalf("ListTasks default query mismatch: %#v", service.lastList)
	}

	var response TaskListResponse
	decodeTaskResponse(t, recorder, &response)
	if len(response.Items) != 1 || response.Items[0].ID != taskID.String() {
		t.Fatalf("unexpected list response: %#v", response)
	}
}

func TestListTasksFiltersParsing(t *testing.T) {
	actorID := testHTTPUUID(1)
	now := testHTTPTime()
	from := now.Add(time.Hour)
	to := now.Add(2 * time.Hour)
	service := &fakeTaskService{}

	path := "/api/v1/tasks?view=assigned&status=IN_PROGRESS&archived=true&overdue=false&deadline_from=" +
		from.Format(time.RFC3339) +
		"&deadline_to=" + to.Format(time.RFC3339) +
		"&search=needle&sort=deadline_asc"
	recorder := performTaskRequest(t, service, actorID, http.MethodGet, path, "", true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	q := service.lastList
	if q.Scope != taskquery.ListScopeAssignedToMe ||
		q.Status == nil ||
		*q.Status != domain.StatusInProgress ||
		q.Archived != taskout.ArchivedOnly ||
		q.Overdue == nil ||
		*q.Overdue ||
		q.Now == nil ||
		!q.Now.Equal(now) ||
		q.DeadlineFrom == nil ||
		!q.DeadlineFrom.Equal(from) ||
		q.DeadlineTo == nil ||
		!q.DeadlineTo.Equal(to) ||
		q.Search != "needle" ||
		q.Sort != taskout.TaskSortDeadlineAtAsc {
		t.Fatalf("ListTasks filters query mismatch: %#v", q)
	}
}

func TestListTasksCreatedView(t *testing.T) {
	actorID := testHTTPUUID(1)
	service := &fakeTaskService{}

	recorder := performTaskRequest(t, service, actorID, http.MethodGet, "/api/v1/tasks?view=created&archived=all&sort=created_asc", "", true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastList.Scope != taskquery.ListScopeCreatedByMe ||
		service.lastList.Archived != taskout.ArchivedAll ||
		service.lastList.Sort != taskout.TaskSortCreatedAtAsc {
		t.Fatalf("ListTasks created query mismatch: %#v", service.lastList)
	}
}

func TestListTasksInvalidFilters(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "view", path: "/api/v1/tasks?view=other"},
		{name: "status", path: "/api/v1/tasks?status=BROKEN"},
		{name: "archived", path: "/api/v1/tasks?archived=banana"},
		{name: "overdue", path: "/api/v1/tasks?overdue=banana"},
		{name: "deadline from", path: "/api/v1/tasks?deadline_from=14.09.2026"},
		{name: "deadline to", path: "/api/v1/tasks?deadline_to=14.09.2026"},
		{name: "sort", path: "/api/v1/tasks?sort=random"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeTaskService{}
			recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodGet, tt.path, "", true)

			assertTaskErrorResponse(t, recorder, http.StatusBadRequest, "invalid_request")
			if service.listCalls != 0 {
				t.Fatalf("ListTasks calls = %d, want 0", service.listCalls)
			}
		})
	}
}

func TestPatchTaskSuccess(t *testing.T) {
	actorID := testHTTPUUID(1)
	taskID := testHTTPUUID(10)
	deadline := testHTTPTime().Add(24 * time.Hour)
	service := &fakeTaskService{
		updateResult: taskDTO(t, taskID, actorID, actorID, domain.StatusOpen, &deadline, nil),
	}

	recorder := performTaskRequest(t, service, actorID, http.MethodPatch, "/api/v1/tasks/"+taskID.String(), `{
		"title": "Updated",
		"description": "updated body",
		"deadline_at": "`+deadline.Format(time.RFC3339)+`"
	}`, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	cmd := service.lastUpdate
	if cmd.ActorID != actorID ||
		cmd.TaskID != taskID ||
		cmd.Title == nil ||
		*cmd.Title != "Updated" ||
		cmd.Description == nil ||
		*cmd.Description != "updated body" ||
		cmd.DeadlineAt == nil ||
		cmd.DeadlineAt.Value == nil ||
		!cmd.DeadlineAt.Value.Equal(deadline) {
		t.Fatalf("UpdateTask command mismatch: %#v", cmd)
	}
}

func TestPatchTaskClearsDeadline(t *testing.T) {
	actorID := testHTTPUUID(1)
	taskID := testHTTPUUID(10)
	service := &fakeTaskService{updateResult: taskDTO(t, taskID, actorID, actorID, domain.StatusOpen, nil, nil)}

	recorder := performTaskRequest(t, service, actorID, http.MethodPatch, "/api/v1/tasks/"+taskID.String(), `{"deadline_at":null}`, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.lastUpdate.DeadlineAt == nil || service.lastUpdate.DeadlineAt.Value != nil {
		t.Fatalf("DeadlineAt command = %#v, want explicit nil", service.lastUpdate.DeadlineAt)
	}
}

func TestPatchTaskAssignee(t *testing.T) {
	actorID := testHTTPUUID(1)
	taskID := testHTTPUUID(10)
	assigneeID := testHTTPUUID(2)
	service := &fakeTaskService{
		updateResult: taskDTO(t, taskID, actorID, assigneeID, domain.StatusOpen, nil, nil),
	}

	recorder := performTaskRequest(t, service, actorID, http.MethodPatch, "/api/v1/tasks/"+taskID.String(), `{
		"assignee_id": "10000000-0000-4000-8000-000000000002"
	}`, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.updateCalls != 1 ||
		service.lastUpdate.ActorID != actorID ||
		service.lastUpdate.TaskID != taskID ||
		service.lastUpdate.AssigneeID == nil ||
		*service.lastUpdate.AssigneeID != assigneeID {
		t.Fatalf("UpdateTask assignee command mismatch: %#v", service.lastUpdate)
	}
}

func TestPatchTaskInvalidAssigneeUUID(t *testing.T) {
	service := &fakeTaskService{}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPatch, "/api/v1/tasks/"+testHTTPUUID(10).String(), `{"assignee_id":"abc"}`, true)

	assertTaskErrorResponse(t, recorder, http.StatusBadRequest, "invalid_request")
	if service.updateCalls != 0 {
		t.Fatalf("UpdateTask calls = %d, want 0", service.updateCalls)
	}
}

func TestPatchTaskEmpty(t *testing.T) {
	service := &fakeTaskService{}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPatch, "/api/v1/tasks/"+testHTTPUUID(10).String(), `{}`, true)

	assertTaskErrorResponse(t, recorder, http.StatusBadRequest, "invalid_request")
	if service.updateCalls != 0 {
		t.Fatalf("UpdateTask calls = %d, want 0", service.updateCalls)
	}
}

func TestPatchTaskForbidden(t *testing.T) {
	service := &fakeTaskService{updateErr: application.ErrTaskAccessDenied}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPatch, "/api/v1/tasks/"+testHTTPUUID(10).String(), `{"title":"Updated"}`, true)

	assertTaskErrorResponse(t, recorder, http.StatusForbidden, "forbidden")
}

func TestPatchTaskAssignmentDenied(t *testing.T) {
	service := &fakeTaskService{updateErr: application.ErrAssignmentDenied}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPatch, "/api/v1/tasks/"+testHTTPUUID(10).String(), `{
		"assignee_id": "10000000-0000-4000-8000-000000000002"
	}`, true)

	assertTaskErrorResponse(t, recorder, http.StatusForbidden, "forbidden")
}

func TestPatchTaskInvalidDomainInput(t *testing.T) {
	service := &fakeTaskService{updateErr: domain.ErrInvalidTitle}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPatch, "/api/v1/tasks/"+testHTTPUUID(10).String(), `{"title":""}`, true)

	assertTaskErrorResponse(t, recorder, http.StatusBadRequest, "invalid_request")
}

func TestCompleteTaskSuccess(t *testing.T) {
	actorID := testHTTPUUID(1)
	taskID := testHTTPUUID(10)
	completedAt := testHTTPTime()
	service := &fakeTaskService{
		changeStatusResult: taskDTO(t, taskID, actorID, actorID, domain.StatusDone, nil, &completedAt),
	}

	recorder := performTaskRequest(t, service, actorID, http.MethodPost, "/api/v1/tasks/"+taskID.String()+"/complete", "", true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.changeStatusCalls != 1 ||
		service.lastChangeStatus.ActorID != actorID ||
		service.lastChangeStatus.TaskID != taskID ||
		service.lastChangeStatus.Status != domain.StatusDone {
		t.Fatalf("ChangeStatus command mismatch: %#v", service.lastChangeStatus)
	}
}

func TestCompleteTaskInvalidTransition(t *testing.T) {
	service := &fakeTaskService{changeStatusErr: domain.ErrInvalidStatusTransition}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks/"+testHTTPUUID(10).String()+"/complete", "", true)

	assertTaskErrorResponse(t, recorder, http.StatusConflict, "invalid_status_transition")
}

func TestArchiveTaskSuccess(t *testing.T) {
	actorID := testHTTPUUID(1)
	taskID := testHTTPUUID(10)
	archivedAt := testHTTPTime()
	service := &fakeTaskService{
		archiveResult: taskDTO(t, taskID, actorID, actorID, domain.StatusOpen, nil, &archivedAt),
	}

	recorder := performTaskRequest(t, service, actorID, http.MethodPost, "/api/v1/tasks/"+taskID.String()+"/archive", "", true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.archiveCalls != 1 || service.lastArchive.ActorID != actorID || service.lastArchive.TaskID != taskID {
		t.Fatalf("ArchiveTask command mismatch: %#v", service.lastArchive)
	}
}

func TestArchiveTaskForbidden(t *testing.T) {
	service := &fakeTaskService{archiveErr: application.ErrTaskAccessDenied}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks/"+testHTTPUUID(10).String()+"/archive", "", true)

	assertTaskErrorResponse(t, recorder, http.StatusForbidden, "forbidden")
}

func TestArchiveTaskNotFound(t *testing.T) {
	service := &fakeTaskService{archiveErr: domain.ErrTaskNotFound}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks/"+testHTTPUUID(10).String()+"/archive", "", true)

	assertTaskErrorResponse(t, recorder, http.StatusNotFound, "task_not_found")
}

func TestArchiveTaskInternalFailure(t *testing.T) {
	service := &fakeTaskService{archiveErr: errTaskHTTPFakeInternal}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks/"+testHTTPUUID(10).String()+"/archive", "", true)

	assertTaskErrorResponse(t, recorder, http.StatusInternalServerError, "internal_server_error")
	if strings.Contains(recorder.Body.String(), "database unavailable") {
		t.Fatalf("internal error response leaked infrastructure details")
	}
}

func TestRestoreTaskSuccess(t *testing.T) {
	actorID := testHTTPUUID(1)
	taskID := testHTTPUUID(10)
	service := &fakeTaskService{
		restoreResult: taskDTO(t, taskID, actorID, actorID, domain.StatusOpen, nil, nil),
	}

	recorder := performTaskRequest(t, service, actorID, http.MethodPost, "/api/v1/tasks/"+taskID.String()+"/restore", "", true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.restoreCalls != 1 || service.lastRestore.ActorID != actorID || service.lastRestore.TaskID != taskID {
		t.Fatalf("RestoreTask command mismatch: %#v", service.lastRestore)
	}
}

func TestRestoreTaskForbidden(t *testing.T) {
	service := &fakeTaskService{restoreErr: application.ErrTaskAccessDenied}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks/"+testHTTPUUID(10).String()+"/restore", "", true)

	assertTaskErrorResponse(t, recorder, http.StatusForbidden, "forbidden")
}

func TestRestoreTaskNotFound(t *testing.T) {
	service := &fakeTaskService{restoreErr: domain.ErrTaskNotFound}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks/"+testHTTPUUID(10).String()+"/restore", "", true)

	assertTaskErrorResponse(t, recorder, http.StatusNotFound, "task_not_found")
}

func TestRestoreTaskInternalFailure(t *testing.T) {
	service := &fakeTaskService{restoreErr: errTaskHTTPFakeInternal}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodPost, "/api/v1/tasks/"+testHTTPUUID(10).String()+"/restore", "", true)

	assertTaskErrorResponse(t, recorder, http.StatusInternalServerError, "internal_server_error")
	if strings.Contains(recorder.Body.String(), "database unavailable") {
		t.Fatalf("internal error response leaked infrastructure details")
	}
}

func TestDeleteTaskEndpointAbsent(t *testing.T) {
	service := &fakeTaskService{}
	recorder := performTaskRequest(t, service, testHTTPUUID(1), http.MethodDelete, "/api/v1/tasks/"+testHTTPUUID(10).String(), "", true)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func performTaskRequest(t *testing.T, service *fakeTaskService, userID uuid.UUID, method string, path string, body string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	tokenValidator := &fakeAccessTokenValidator{userID: userID}
	authMiddleware, err := authhttp.NewAuthMiddleware(tokenValidator)
	if err != nil {
		t.Fatalf("new auth middleware: %v", err)
	}
	handler := NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler.now = func() time.Time {
		return testHTTPTime()
	}
	RegisterRoutes(mux, handler, authMiddleware.Authenticate)

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.Header.Set("Authorization", "Bearer valid-token")
	}

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	return recorder
}

func assertTaskErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, status, recorder.Body.String())
	}
	assertTaskJSONContentType(t, recorder)

	var response ErrorResponse
	decodeTaskResponse(t, recorder, &response)
	if response.Error.Code != code {
		t.Fatalf("error code = %q, want %q", response.Error.Code, code)
	}
}

func assertTaskJSONContentType(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	contentType := recorder.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
}

func decodeTaskResponse(t *testing.T, recorder *httptest.ResponseRecorder, dst any) {
	t.Helper()

	if err := json.Unmarshal(recorder.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v: %s", err, recorder.Body.String())
	}
}

type fakeAccessTokenValidator struct {
	userID uuid.UUID
}

func (v *fakeAccessTokenValidator) ValidateAccessToken(token string) (authout.AccessTokenClaims, error) {
	if token != "valid-token" {
		return authout.AccessTokenClaims{}, errors.New("invalid token")
	}

	return authout.AccessTokenClaims{UserID: v.userID}, nil
}

type fakeTaskService struct {
	createResult domain.Task
	createErr    error
	createCalls  int
	lastCreate   command.CreateTaskCommand

	getResult domain.Task
	getErr    error
	getCalls  int
	lastGet   taskquery.GetTaskQuery

	listResult []domain.Task
	listErr    error
	listCalls  int
	lastList   taskquery.ListTasksQuery

	updateResult domain.Task
	updateErr    error
	updateCalls  int
	lastUpdate   command.UpdateTaskCommand

	changeStatusResult domain.Task
	changeStatusErr    error
	changeStatusCalls  int
	lastChangeStatus   command.ChangeStatusCommand

	archiveResult domain.Task
	archiveErr    error
	archiveCalls  int
	lastArchive   command.ArchiveTaskCommand

	restoreResult domain.Task
	restoreErr    error
	restoreCalls  int
	lastRestore   command.RestoreTaskCommand
}

func (s *fakeTaskService) CreateTask(_ context.Context, cmd command.CreateTaskCommand) (domain.Task, error) {
	s.createCalls++
	s.lastCreate = cmd
	if s.createErr != nil {
		return domain.Task{}, s.createErr
	}

	return s.createResult, nil
}

func (s *fakeTaskService) GetTask(_ context.Context, q taskquery.GetTaskQuery) (domain.Task, error) {
	s.getCalls++
	s.lastGet = q
	if s.getErr != nil {
		return domain.Task{}, s.getErr
	}

	return s.getResult, nil
}

func (s *fakeTaskService) ListTasks(_ context.Context, q taskquery.ListTasksQuery) ([]domain.Task, error) {
	s.listCalls++
	s.lastList = q
	if s.listErr != nil {
		return nil, s.listErr
	}

	return s.listResult, nil
}

func (s *fakeTaskService) UpdateTask(_ context.Context, cmd command.UpdateTaskCommand) (domain.Task, error) {
	s.updateCalls++
	s.lastUpdate = cmd
	if s.updateErr != nil {
		return domain.Task{}, s.updateErr
	}

	return s.updateResult, nil
}

func (s *fakeTaskService) ReassignTask(_ context.Context, cmd command.ReassignTaskCommand) (domain.Task, error) {
	return domain.Task{}, nil
}

func (s *fakeTaskService) ChangeStatus(_ context.Context, cmd command.ChangeStatusCommand) (domain.Task, error) {
	s.changeStatusCalls++
	s.lastChangeStatus = cmd
	if s.changeStatusErr != nil {
		return domain.Task{}, s.changeStatusErr
	}

	return s.changeStatusResult, nil
}

func (s *fakeTaskService) ArchiveTask(_ context.Context, cmd command.ArchiveTaskCommand) (domain.Task, error) {
	s.archiveCalls++
	s.lastArchive = cmd
	if s.archiveErr != nil {
		return domain.Task{}, s.archiveErr
	}

	return s.archiveResult, nil
}

func (s *fakeTaskService) RestoreTask(_ context.Context, cmd command.RestoreTaskCommand) (domain.Task, error) {
	s.restoreCalls++
	s.lastRestore = cmd
	if s.restoreErr != nil {
		return domain.Task{}, s.restoreErr
	}

	return s.restoreResult, nil
}

func taskDTO(t *testing.T, id uuid.UUID, creatorID uuid.UUID, assigneeID uuid.UUID, status domain.Status, deadlineAt *time.Time, timestamp *time.Time) domain.Task {
	t.Helper()

	now := testHTTPTime()
	var completedAt *time.Time
	var archivedAt *time.Time
	if status.IsCompleted() {
		completedAt = timestamp
	}
	if timestamp != nil && !status.IsCompleted() {
		archivedAt = timestamp
	}

	task, err := domain.RestoreTask(
		id,
		nil,
		creatorID,
		assigneeID,
		"Task A",
		"body",
		status,
		deadlineAt,
		now.Add(-time.Hour),
		now,
		completedAt,
		archivedAt,
	)
	if err != nil {
		t.Fatalf("RestoreTask: %v", err)
	}

	return task
}

func testHTTPTime() time.Time {
	return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
}

func testHTTPUUID(n int) uuid.UUID {
	return uuid.MustParse("10000000-0000-4000-8000-" + leftPadHTTPInt(n, 12))
}

func leftPadHTTPInt(n int, width int) string {
	value := ""
	for n > 0 {
		value = string(rune('0'+n%10)) + value
		n /= 10
	}
	for len(value) < width {
		value = "0" + value
	}

	return value
}
