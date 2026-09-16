package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	reminderhttp "go_pet_project/internal/reminder/adapter/in/http"
	reminderdomain "go_pet_project/internal/reminder/domain"
)

func TestReminderHTTPE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_reminder_http_e2e_test_")
	router, _ := buildTaskHTTPRouter(t, pg)

	tokens := registerAndLoginTaskUser(t, router, "reminder-http@example.com", "reminder_http", "plain-password")
	otherTokens := registerAndLoginTaskUser(t, router, "reminder-http-other@example.com", "reminder_http_other", "plain-password")

	deadline := time.Date(2035, 9, 20, 16, 0, 0, 0, time.UTC)
	task := createReminderHTTPTask(t, router, tokens.AccessToken, "Reminder HTTP task", &deadline)
	otherTask := createReminderHTTPTask(t, router, tokens.AccessToken, "Other reminder HTTP task", &deadline)

	noAuth := request(t, router, http.MethodGet, "/api/v1/tasks/"+task.ID+"/reminders", nil, nil)
	if noAuth.Code != http.StatusUnauthorized {
		t.Fatalf("no auth status = %d, want 401: %s", noAuth.Code, noAuth.Body.String())
	}

	absoluteAt := time.Date(2035, 9, 19, 16, 0, 0, 0, time.UTC)
	createAbsolute := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks/"+task.ID+"/reminders", map[string]any{
		"kind":       string(reminderdomain.KindAbsolute),
		"trigger_at": absoluteAt.Format(time.RFC3339),
	}, tokens.AccessToken)
	if createAbsolute.Code != http.StatusCreated {
		t.Fatalf("create absolute status = %d, want 201: %s", createAbsolute.Code, createAbsolute.Body.String())
	}
	absolute := decodeReminderHTTPResponse(t, createAbsolute)
	if absolute.Kind != string(reminderdomain.KindAbsolute) || !absolute.TriggerAt.Equal(absoluteAt) {
		t.Fatalf("absolute response mismatch: %#v", absolute)
	}

	createRelative := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks/"+task.ID+"/reminders", map[string]any{
		"kind":           string(reminderdomain.KindBeforeDeadline),
		"offset_seconds": 7200,
	}, tokens.AccessToken)
	if createRelative.Code != http.StatusCreated {
		t.Fatalf("create relative status = %d, want 201: %s", createRelative.Code, createRelative.Body.String())
	}
	relative := decodeReminderHTTPResponse(t, createRelative)
	if relative.OffsetSeconds == nil ||
		*relative.OffsetSeconds != 7200 ||
		!relative.TriggerAt.Equal(deadline.Add(-2*time.Hour)) {
		t.Fatalf("relative response mismatch: %#v", relative)
	}

	list := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks/"+task.ID+"/reminders", nil, tokens.AccessToken)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", list.Code, list.Body.String())
	}
	assertReminderHTTPListContains(t, list, absolute.ID)
	assertReminderHTTPListContains(t, list, relative.ID)

	nextAbsoluteAt := absoluteAt.Add(time.Hour)
	patch := authenticatedJSONRequest(t, router, http.MethodPatch, "/api/v1/tasks/"+task.ID+"/reminders/"+absolute.ID, map[string]any{
		"trigger_at": nextAbsoluteAt.Format(time.RFC3339),
	}, tokens.AccessToken)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", patch.Code, patch.Body.String())
	}
	if patched := decodeReminderHTTPResponse(t, patch); !patched.TriggerAt.Equal(nextAbsoluteAt) {
		t.Fatalf("patched TriggerAt = %s, want %s", patched.TriggerAt, nextAbsoluteAt)
	}

	deleteReminder := authenticatedRequest(t, router, http.MethodDelete, "/api/v1/tasks/"+task.ID+"/reminders/"+relative.ID, nil, tokens.AccessToken)
	if deleteReminder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204: %s", deleteReminder.Code, deleteReminder.Body.String())
	}

	forbidden := authenticatedRequest(t, router, http.MethodGet, "/api/v1/tasks/"+task.ID+"/reminders", nil, otherTokens.AccessToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d, want 403: %s", forbidden.Code, forbidden.Body.String())
	}

	nestedMismatch := authenticatedJSONRequest(t, router, http.MethodPatch, "/api/v1/tasks/"+otherTask.ID+"/reminders/"+absolute.ID, map[string]any{
		"trigger_at": nextAbsoluteAt.Add(time.Hour).Format(time.RFC3339),
	}, tokens.AccessToken)
	if nestedMismatch.Code != http.StatusNotFound {
		t.Fatalf("nested mismatch status = %d, want 404: %s", nestedMismatch.Code, nestedMismatch.Body.String())
	}

	invalid := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks/"+task.ID+"/reminders", map[string]any{
		"kind":           string(reminderdomain.KindBeforeDeadline),
		"offset_seconds": 7200,
		"trigger_at":     absoluteAt.Format(time.RFC3339),
	}, tokens.AccessToken)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want 400: %s", invalid.Code, invalid.Body.String())
	}
}

func createReminderHTTPTask(t *testing.T, router http.Handler, accessToken string, title string, deadline *time.Time) taskHTTPResponseForReminder {
	t.Helper()

	body := map[string]any{"title": title}
	if deadline != nil {
		body["deadline_at"] = deadline.Format(time.RFC3339)
	}
	create := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", body, accessToken)
	if create.Code != http.StatusCreated {
		t.Fatalf("create task status = %d, want 201: %s", create.Code, create.Body.String())
	}

	var response taskHTTPResponseForReminder
	if err := json.Unmarshal(create.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode task response: %v: %s", err, create.Body.String())
	}

	return response
}

type taskHTTPResponseForReminder struct {
	ID string `json:"id"`
}

func decodeReminderHTTPResponse(t *testing.T, recorder *httptest.ResponseRecorder) reminderhttp.ReminderResponse {
	t.Helper()

	var response reminderhttp.ReminderResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode reminder response: %v: %s", err, recorder.Body.String())
	}

	return response
}

func decodeReminderHTTPListResponse(t *testing.T, recorder *httptest.ResponseRecorder) reminderhttp.ReminderListResponse {
	t.Helper()

	var response reminderhttp.ReminderListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode reminder list response: %v: %s", err, recorder.Body.String())
	}

	return response
}

func assertReminderHTTPListContains(t *testing.T, recorder *httptest.ResponseRecorder, id string) {
	t.Helper()

	for _, item := range decodeReminderHTTPListResponse(t, recorder).Items {
		if item.ID == id {
			return
		}
	}
	t.Fatalf("reminder list does not contain %s: %s", id, recorder.Body.String())
}
