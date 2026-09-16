package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	authjwt "go_pet_project/internal/auth/adapter/out/jwt"
	authpassword "go_pet_project/internal/auth/adapter/out/password"
	authpostgres "go_pet_project/internal/auth/adapter/out/postgres"
	authrefresh "go_pet_project/internal/auth/adapter/out/refresh"
	authservice "go_pet_project/internal/auth/application/service"
	notificationhttp "go_pet_project/internal/notification/adapter/in/http"
	notificationpostgres "go_pet_project/internal/notification/adapter/out/postgres"
	notificationservice "go_pet_project/internal/notification/application/service"
	permissionpostgres "go_pet_project/internal/permission/adapter/out/postgres"
	permissionservice "go_pet_project/internal/permission/application/service"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/migrations"
	reminderhttp "go_pet_project/internal/reminder/adapter/in/http"
	reminderpostgres "go_pet_project/internal/reminder/adapter/out/postgres"
	reminderservice "go_pet_project/internal/reminder/application/service"
	taskhttp "go_pet_project/internal/task/adapter/in/http"
	taskpostgres "go_pet_project/internal/task/adapter/out/postgres"
	taskservice "go_pet_project/internal/task/application/service"
	taskdomain "go_pet_project/internal/task/domain"
	userhttp "go_pet_project/internal/user/adapter/in/http"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"
	userservice "go_pet_project/internal/user/application/service"

	"github.com/google/uuid"
)

func TestTaskHTTPE2ESelfTaskFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_task_http_e2e_test_")
	router, _ := buildTaskHTTPRouter(t, pg)

	tokens := registerAndLoginTaskUser(t, router, "task-self@example.com", "task_self", "plain-password")

	create := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":       "Task A",
		"description": "first body",
	}, tokens.AccessToken)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", create.Code, create.Body.String())
	}
	created := decodeTaskHTTPResponse(t, create)
	if created.ID == "" || created.Title != "Task A" || created.CreatorID != created.AssigneeID {
		t.Fatalf("unexpected create response: %#v", created)
	}

	get := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks/"+created.ID, nil, tokens.AccessToken)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", get.Code, get.Body.String())
	}

	list := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks", nil, tokens.AccessToken)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", list.Code, list.Body.String())
	}
	assertTaskHTTPListContains(t, list, created.ID)

	patch := authenticatedJSONRequest(t, router, http.MethodPatch, "/api/v1/tasks/"+created.ID, map[string]any{
		"title": "Updated",
	}, tokens.AccessToken)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patch.Code, patch.Body.String())
	}

	getUpdated := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks/"+created.ID, nil, tokens.AccessToken)
	updated := decodeTaskHTTPResponse(t, getUpdated)
	if updated.Title != "Updated" {
		t.Fatalf("updated title = %q, want Updated", updated.Title)
	}

	complete := authenticatedRequest(t, router, http.MethodPost, "/api/v1/tasks/"+created.ID+"/complete", nil, tokens.AccessToken)
	if complete.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want 200: %s", complete.Code, complete.Body.String())
	}
	completed := decodeTaskHTTPResponse(t, complete)
	if completed.Status != string(taskdomain.StatusDone) || completed.CompletedAt == nil {
		t.Fatalf("completed response mismatch: %#v", completed)
	}

	archive := authenticatedRequest(t, router, http.MethodPost, "/api/v1/tasks/"+created.ID+"/archive", nil, tokens.AccessToken)
	if archive.Code != http.StatusOK {
		t.Fatalf("archive status = %d, want 200: %s", archive.Code, archive.Body.String())
	}
	archived := decodeTaskHTTPResponse(t, archive)
	if archived.ArchivedAt == nil {
		t.Fatalf("archived response should contain archived_at")
	}

	defaultList := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks", nil, tokens.AccessToken)
	assertTaskHTTPListNotContains(t, defaultList, created.ID)

	getArchived := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks/"+created.ID, nil, tokens.AccessToken)
	if getArchived.Code != http.StatusOK {
		t.Fatalf("get archived status = %d, want 200: %s", getArchived.Code, getArchived.Body.String())
	}
	if decodeTaskHTTPResponse(t, getArchived).ArchivedAt == nil {
		t.Fatalf("GET by ID must return archived task")
	}

	archivedList := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks?archived=true", nil, tokens.AccessToken)
	assertTaskHTTPListContains(t, archivedList, created.ID)

	restore := authenticatedRequest(t, router, http.MethodPost, "/api/v1/tasks/"+created.ID+"/restore", nil, tokens.AccessToken)
	if restore.Code != http.StatusOK {
		t.Fatalf("restore status = %d, want 200: %s", restore.Code, restore.Body.String())
	}
	if decodeTaskHTTPResponse(t, restore).ArchivedAt != nil {
		t.Fatalf("restore response must clear archived_at")
	}

	restoredDefaultList := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks", nil, tokens.AccessToken)
	assertTaskHTTPListContains(t, restoredDefaultList, created.ID)

	laterDeadline := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	earlierDeadline := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	later := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":       "Needle later",
		"deadline_at": laterDeadline.Format(time.RFC3339),
	}, tokens.AccessToken)
	if later.Code != http.StatusCreated {
		t.Fatalf("create later search task status = %d, want 201: %s", later.Code, later.Body.String())
	}
	earlier := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":       "Needle earlier",
		"deadline_at": earlierDeadline.Format(time.RFC3339),
	}, tokens.AccessToken)
	if earlier.Code != http.StatusCreated {
		t.Fatalf("create earlier search task status = %d, want 201: %s", earlier.Code, earlier.Body.String())
	}
	searchSort := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks?search=needle&sort=deadline_asc", nil, tokens.AccessToken)
	assertTaskHTTPListOrder(t, searchSort, decodeTaskHTTPResponse(t, earlier).ID, decodeTaskHTTPResponse(t, later).ID)
}

func TestTaskHTTPE2ETwoUserAccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_task_http_access_e2e_test_")
	router, _ := buildTaskHTTPRouter(t, pg)

	tokensA := registerAndLoginTaskUser(t, router, "task-a@example.com", "task_a", "plain-password")
	tokensB := registerAndLoginTaskUser(t, router, "task-b@example.com", "task_b", "plain-password")
	tokensC := registerAndLoginTaskUser(t, router, "task-c@example.com", "task_c", "plain-password")
	userA := currentTaskHTTPUser(t, router, tokensA.AccessToken)
	userB := currentTaskHTTPUser(t, router, tokensB.AccessToken)
	userC := currentTaskHTTPUser(t, router, tokensC.AccessToken)
	insertAssignmentPermission(ctx, t, pg.SQL, userA.ID, userB.ID)

	assignableA := authenticatedRequest(t, router, http.MethodGet, "/api/v1/users/assignable", nil, tokensA.AccessToken)
	if assignableA.Code != http.StatusOK {
		t.Fatalf("assignable A status = %d, want 200: %s", assignableA.Code, assignableA.Body.String())
	}
	assertAssignableUsers(t, assignableA, []string{userA.ID, userB.ID}, []string{userC.ID})

	assignableB := authenticatedRequest(t, router, http.MethodGet, "/api/v1/users/assignable", nil, tokensB.AccessToken)
	if assignableB.Code != http.StatusOK {
		t.Fatalf("assignable B status = %d, want 200: %s", assignableB.Code, assignableB.Body.String())
	}
	assertAssignableUsers(t, assignableB, []string{userB.ID}, []string{userA.ID})

	createForB := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", map[string]any{
		"assignee_id": userB.ID,
		"title":       "Task for B",
	}, tokensA.AccessToken)
	if createForB.Code != http.StatusCreated {
		t.Fatalf("create A->B status = %d, want 201: %s", createForB.Code, createForB.Body.String())
	}
	createdForB := decodeTaskHTTPResponse(t, createForB)
	if createdForB.CreatorID != userA.ID || createdForB.AssigneeID != userB.ID {
		t.Fatalf("created A->B identities mismatch: %#v", createdForB)
	}

	createForC := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", map[string]any{
		"assignee_id": userC.ID,
		"title":       "Task for C denied",
	}, tokensA.AccessToken)
	if createForC.Code != http.StatusForbidden {
		t.Fatalf("create A->C status = %d, want 403: %s", createForC.Code, createForC.Body.String())
	}
	assertTaskTitleCount(t, pg, "Task for C denied", 0)

	createBForA := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", map[string]any{
		"assignee_id": userA.ID,
		"title":       "Task for A denied",
	}, tokensB.AccessToken)
	if createBForA.Code != http.StatusForbidden {
		t.Fatalf("create B->A status = %d, want 403: %s", createBForA.Code, createBForA.Body.String())
	}

	path := "/api/v1/tasks/" + createdForB.ID
	getByCreator := authenticatedRequest(t, router, http.MethodGet, path, nil, tokensA.AccessToken)
	if getByCreator.Code != http.StatusOK {
		t.Fatalf("creator get status = %d, want 200: %s", getByCreator.Code, getByCreator.Body.String())
	}

	patchByCreator := authenticatedJSONRequest(t, router, http.MethodPatch, path, map[string]any{"title": "creator updated"}, tokensA.AccessToken)
	if patchByCreator.Code != http.StatusOK {
		t.Fatalf("creator patch status = %d, want 200: %s", patchByCreator.Code, patchByCreator.Body.String())
	}

	getByAssignee := authenticatedRequest(t, router, http.MethodGet, path, nil, tokensB.AccessToken)
	if getByAssignee.Code != http.StatusOK {
		t.Fatalf("assignee get status = %d, want 200: %s", getByAssignee.Code, getByAssignee.Body.String())
	}

	completeByAssignee := authenticatedRequest(t, router, http.MethodPost, path+"/complete", nil, tokensB.AccessToken)
	if completeByAssignee.Code != http.StatusOK {
		t.Fatalf("assignee complete status = %d, want 200: %s", completeByAssignee.Code, completeByAssignee.Body.String())
	}

	patchByAssignee := authenticatedJSONRequest(t, router, http.MethodPatch, path, map[string]any{"title": "assignee forbidden"}, tokensB.AccessToken)
	if patchByAssignee.Code != http.StatusForbidden {
		t.Fatalf("assignee patch status = %d, want 403: %s", patchByAssignee.Code, patchByAssignee.Body.String())
	}

	archiveByAssignee := authenticatedRequest(t, router, http.MethodPost, path+"/archive", nil, tokensB.AccessToken)
	if archiveByAssignee.Code != http.StatusForbidden {
		t.Fatalf("assignee archive status = %d, want 403: %s", archiveByAssignee.Code, archiveByAssignee.Body.String())
	}

	archiveByCreator := authenticatedRequest(t, router, http.MethodPost, path+"/archive", nil, tokensA.AccessToken)
	if archiveByCreator.Code != http.StatusOK {
		t.Fatalf("creator archive status = %d, want 200: %s", archiveByCreator.Code, archiveByCreator.Body.String())
	}

	getByUnrelated := authenticatedRequest(t, router, http.MethodGet, path, nil, tokensC.AccessToken)
	if getByUnrelated.Code != http.StatusForbidden {
		t.Fatalf("unrelated get status = %d, want 403: %s", getByUnrelated.Code, getByUnrelated.Body.String())
	}

	reassignCandidate := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", map[string]any{
		"assignee_id": userB.ID,
		"title":       "Task to reassign",
	}, tokensA.AccessToken)
	if reassignCandidate.Code != http.StatusCreated {
		t.Fatalf("create reassign candidate status = %d, want 201: %s", reassignCandidate.Code, reassignCandidate.Body.String())
	}
	reassignTask := decodeTaskHTTPResponse(t, reassignCandidate)

	insertAssignmentPermission(ctx, t, pg.SQL, userA.ID, userC.ID)
	reassignToC := authenticatedJSONRequest(t, router, http.MethodPatch, "/api/v1/tasks/"+reassignTask.ID, map[string]any{
		"assignee_id": userC.ID,
	}, tokensA.AccessToken)
	if reassignToC.Code != http.StatusOK {
		t.Fatalf("reassign B->C status = %d, want 200: %s", reassignToC.Code, reassignToC.Body.String())
	}
	reassigned := decodeTaskHTTPResponse(t, reassignToC)
	if reassigned.CreatorID != userA.ID || reassigned.AssigneeID != userC.ID || reassigned.Status != reassignTask.Status {
		t.Fatalf("reassigned task mismatch: %#v", reassigned)
	}

	oldAssigneeGet := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks/"+reassignTask.ID, nil, tokensB.AccessToken)
	if oldAssigneeGet.Code != http.StatusForbidden {
		t.Fatalf("old assignee get status = %d, want 403: %s", oldAssigneeGet.Code, oldAssigneeGet.Body.String())
	}

	newAssigneeGet := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks/"+reassignTask.ID, nil, tokensC.AccessToken)
	if newAssigneeGet.Code != http.StatusOK {
		t.Fatalf("new assignee get status = %d, want 200: %s", newAssigneeGet.Code, newAssigneeGet.Body.String())
	}

	newAssigneeComplete := authenticatedRequest(t, router, http.MethodPost, "/api/v1/tasks/"+reassignTask.ID+"/complete", nil, tokensC.AccessToken)
	if newAssigneeComplete.Code != http.StatusOK {
		t.Fatalf("new assignee complete status = %d, want 200: %s", newAssigneeComplete.Code, newAssigneeComplete.Body.String())
	}
}

func setupTaskHTTPDatabase(ctx context.Context, t *testing.T, prefix string) *database.Postgres {
	t.Helper()

	cfg := loadConfig(t)
	startPostgres(ctx, t)
	waitForPostgres(ctx, t, cfg.DB)

	testDBName := prefix + uuid.NewString()
	adminDB := openAdminDB(ctx, t, cfg.DB)
	createTestDatabase(ctx, t, adminDB, testDBName)
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer dropCancel()
		dropTestDatabase(dropCtx, t, adminDB, testDBName)
		adminDB.Close()
	})

	testCfg := cfg.DB
	testCfg.Name = testDBName

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrations.Up(ctx, testCfg, migrationsDir, logger); err != nil {
		t.Fatalf("migration up: %v", err)
	}

	pg, err := database.Open(ctx, testCfg, logger)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() {
		pg.Close()
	})

	return pg
}

func buildTaskHTTPRouter(t *testing.T, pg *database.Postgres) (http.Handler, *taskpostgres.Repository) {
	t.Helper()

	passwordHasher, err := authpassword.NewHasher(4)
	if err != nil {
		t.Fatalf("new password hasher: %v", err)
	}
	tokenProvider, err := authjwt.NewProvider("task-e2e-jwt-secret-with-at-least-32-characters", 15*time.Minute)
	if err != nil {
		t.Fatalf("new token provider: %v", err)
	}
	refreshTokenGenerator, err := authrefresh.NewTokenGenerator(authrefresh.DefaultTokenBytes)
	if err != nil {
		t.Fatalf("new refresh token generator: %v", err)
	}

	userRepository := userpostgres.NewRepository(pg.GORM)
	refreshTokenRepository := authpostgres.NewRefreshTokenRepository(pg.GORM)
	authService, err := authservice.NewAuthService(
		userRepository,
		passwordHasher,
		tokenProvider,
		refreshTokenRepository,
		refreshTokenGenerator,
		30*24*time.Hour,
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	authMiddleware, err := authhttp.NewAuthMiddleware(tokenProvider)
	if err != nil {
		t.Fatalf("new auth middleware: %v", err)
	}

	permissionRepository := permissionpostgres.NewRepository(pg.GORM)
	permissionService, err := permissionservice.NewPermissionService(permissionRepository)
	if err != nil {
		t.Fatalf("new permission service: %v", err)
	}

	userService, err := userservice.NewUserService(userRepository, permissionService)
	if err != nil {
		t.Fatalf("new user service: %v", err)
	}

	taskRepository := taskpostgres.NewRepository(pg.GORM)
	reminderRepository := reminderpostgres.NewRepository(pg.GORM)
	reminderService, err := reminderservice.NewReminderService(taskRepository, reminderRepository)
	if err != nil {
		t.Fatalf("new reminder service: %v", err)
	}
	transactionRunner := database.NewTransactionRunner(pg.GORM)
	taskService, err := taskservice.NewTaskService(taskRepository, permissionService, reminderService, transactionRunner)
	if err != nil {
		t.Fatalf("new task service: %v", err)
	}
	notificationRepository := notificationpostgres.NewNotificationRepository(pg.GORM)
	processedEventRepository := notificationpostgres.NewProcessedEventRepository(pg.GORM)
	notificationService, err := notificationservice.NewNotificationService(notificationRepository, processedEventRepository, transactionRunner, cfgKafkaConsumerGroupForTests())
	if err != nil {
		t.Fatalf("new notification service: %v", err)
	}

	mux := http.NewServeMux()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authhttp.RegisterRoutes(mux, authhttp.NewHandler(authService, logger))
	userhttp.RegisterRoutes(mux, userhttp.NewHandler(userService, logger), authMiddleware.Authenticate)
	taskhttp.RegisterRoutes(mux, taskhttp.NewHandler(taskService, logger), authMiddleware.Authenticate)
	reminderhttp.RegisterRoutes(mux, reminderhttp.NewHandler(reminderService, logger), authMiddleware.Authenticate)
	notificationhttp.RegisterRoutes(mux, notificationhttp.NewHandler(notificationService, logger), authMiddleware.Authenticate)

	return mux, taskRepository
}

func cfgKafkaConsumerGroupForTests() string {
	return "todo-notifier-v1"
}

func registerAndLoginTaskUser(t *testing.T, router http.Handler, email string, username string, password string) authhttp.TokenResponse {
	t.Helper()

	register := requestJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":    email,
		"username": username,
		"password": password,
		"timezone": "UTC",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("register %s status = %d, want 201: %s", email, register.Code, register.Body.String())
	}

	login := requestJSON(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": password,
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login %s status = %d, want 200: %s", email, login.Code, login.Body.String())
	}

	return decodeTokenResponse(t, login)
}

func currentTaskHTTPUser(t *testing.T, router http.Handler, accessToken string) userhttp.GetMeResponse {
	t.Helper()

	me := authenticatedRequest(t, router, http.MethodGet, "/api/v1/users/me", nil, accessToken)
	if me.Code != http.StatusOK {
		t.Fatalf("users/me status = %d, want 200: %s", me.Code, me.Body.String())
	}

	return decodeGetMeResponse(t, me)
}

func authenticatedJSONRequest(t *testing.T, handler http.Handler, method string, path string, body any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	return authenticatedRequest(t, handler, method, path, payload, accessToken, "Content-Type", "application/json")
}

func authenticatedRequest(t *testing.T, handler http.Handler, method string, path string, payload []byte, accessToken string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()

	headerMap := map[string]string{"Authorization": "Bearer " + accessToken}
	for i := 0; i+1 < len(headers); i += 2 {
		headerMap[headers[i]] = headers[i+1]
	}

	return request(t, handler, method, path, payload, headerMap)
}

func decodeTaskHTTPResponse(t *testing.T, recorder *httptest.ResponseRecorder) taskhttp.TaskResponse {
	t.Helper()

	var response taskhttp.TaskResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode task response: %v: %s", err, recorder.Body.String())
	}

	return response
}

func decodeTaskHTTPListResponse(t *testing.T, recorder *httptest.ResponseRecorder) taskhttp.TaskListResponse {
	t.Helper()

	var response taskhttp.TaskListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode task list response: %v: %s", err, recorder.Body.String())
	}

	return response
}

func assertTaskHTTPListContains(t *testing.T, recorder *httptest.ResponseRecorder, id string) {
	t.Helper()

	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	for _, item := range decodeTaskHTTPListResponse(t, recorder).Items {
		if item.ID == id {
			return
		}
	}

	t.Fatalf("expected task %s in list: %s", id, recorder.Body.String())
}

func assertTaskHTTPListNotContains(t *testing.T, recorder *httptest.ResponseRecorder, id string) {
	t.Helper()

	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	for _, item := range decodeTaskHTTPListResponse(t, recorder).Items {
		if item.ID == id {
			t.Fatalf("did not expect task %s in list: %s", id, recorder.Body.String())
		}
	}
}

func assertTaskHTTPListOrder(t *testing.T, recorder *httptest.ResponseRecorder, wantIDs ...string) {
	t.Helper()

	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	items := decodeTaskHTTPListResponse(t, recorder).Items
	if len(items) != len(wantIDs) {
		t.Fatalf("list item count = %d, want %d: %s", len(items), len(wantIDs), recorder.Body.String())
	}
	for i, wantID := range wantIDs {
		if items[i].ID != wantID {
			t.Fatalf("list item %d = %s, want %s: %s", i, items[i].ID, wantID, recorder.Body.String())
		}
	}
}

func assertAssignableUsers(t *testing.T, recorder *httptest.ResponseRecorder, wantPresent []string, wantAbsent []string) {
	t.Helper()

	var response userhttp.AssignableUsersResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode assignable users response: %v: %s", err, recorder.Body.String())
	}

	seen := make(map[string]bool, len(response.Items))
	for _, item := range response.Items {
		seen[item.ID] = true
		if item.Email == "" || item.Username == "" || item.Timezone == "" {
			t.Fatalf("assignable user response missing safe fields: %#v", item)
		}
	}
	for _, id := range wantPresent {
		if !seen[id] {
			t.Fatalf("expected assignable user %s in response: %s", id, recorder.Body.String())
		}
	}
	for _, id := range wantAbsent {
		if seen[id] {
			t.Fatalf("did not expect assignable user %s in response: %s", id, recorder.Body.String())
		}
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("password")) || bytes.Contains(recorder.Body.Bytes(), []byte("password_hash")) {
		t.Fatalf("assignable users response leaked password data: %s", recorder.Body.String())
	}
}

func assertTaskTitleCount(t *testing.T, pg *database.Postgres, title string, want int) {
	t.Helper()

	var count int
	if err := pg.SQL.QueryRow(`SELECT COUNT(*) FROM tasks WHERE title = $1`, title).Scan(&count); err != nil {
		t.Fatalf("count task title %q: %v", title, err)
	}
	if count != want {
		t.Fatalf("task title %q count = %d, want %d", title, count, want)
	}
}
