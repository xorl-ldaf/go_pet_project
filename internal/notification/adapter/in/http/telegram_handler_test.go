package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	authout "go_pet_project/internal/auth/application/port/out"
	"go_pet_project/internal/notification/application/command"
	"go_pet_project/internal/notification/application/query"
	"go_pet_project/internal/notification/domain"

	"github.com/google/uuid"
)

func TestTelegramAndNotificationSettingsHTTP(t *testing.T) {
	actorID := uuid.New()
	service := &fakeNotificationService{
		tokenResult: command.TelegramLinkTokenResult{
			Token:     "raw-token",
			ExpiresAt: time.Date(2026, 9, 15, 12, 10, 0, 0, time.UTC),
		},
		settings: query.NotificationSettings{
			Internal: true,
			Telegram: query.TelegramSettings{
				Linked:  true,
				Enabled: true,
			},
		},
	}
	router := newTestRouter(t, actorID, service)

	tokenResponse := doJSON[TelegramLinkTokenResponse](t, router, http.MethodPost, "/api/v1/users/me/telegram/link", nil, http.StatusOK)
	if tokenResponse.Token != "raw-token" || tokenResponse.DeepLinkURL == nil || *tokenResponse.DeepLinkURL != "https://t.me/test_bot?start=raw-token" {
		t.Fatalf("token response = %+v", tokenResponse)
	}

	settingsResponse := doJSON[NotificationSettingsResponse](t, router, http.MethodGet, "/api/v1/users/me/notification-settings", nil, http.StatusOK)
	if !settingsResponse.Internal || !settingsResponse.Telegram.Linked || !settingsResponse.Telegram.Enabled {
		t.Fatalf("settings response = %+v", settingsResponse)
	}

	disabled := false
	service.settings.Telegram.Enabled = false
	patched := doJSON[NotificationSettingsResponse](t, router, http.MethodPatch, "/api/v1/users/me/notification-settings", map[string]any{
		"telegram": map[string]any{"enabled": disabled},
	}, http.StatusOK)
	if patched.Telegram.Enabled {
		t.Fatalf("patched settings enabled = true, want false")
	}
	if service.enabled == nil || *service.enabled {
		t.Fatalf("service enabled = %v, want false", service.enabled)
	}

	rr := doRequest(t, router, http.MethodDelete, "/api/v1/users/me/telegram/link", nil, true)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204; body=%s", rr.Code, rr.Body.String())
	}
	if !service.deleted {
		t.Fatal("DeleteTelegramLink was not called")
	}
}

func TestTelegramHTTPNoAuth(t *testing.T) {
	router := newTestRouter(t, uuid.New(), &fakeNotificationService{})

	rr := doRequest(t, router, http.MethodPost, "/api/v1/users/me/telegram/link", nil, false)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("POST no auth status = %d, want 401", rr.Code)
	}
}

func newTestRouter(t *testing.T, actorID uuid.UUID, service *fakeNotificationService) http.Handler {
	t.Helper()
	authMiddleware, err := authhttp.NewAuthMiddleware(fakeTokenValidator{actorID: actorID})
	if err != nil {
		t.Fatalf("NewAuthMiddleware: %v", err)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewHandler(service, nil, "test_bot"), authMiddleware.Authenticate)

	return mux
}

func doJSON[T any](t *testing.T, router http.Handler, method string, path string, body any, wantStatus int) T {
	t.Helper()
	rr := doRequest(t, router, method, path, body, true)
	if rr.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, rr.Code, wantStatus, rr.Body.String())
	}
	var response T
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return response
}

func doRequest(t *testing.T, router http.Handler, method string, path string, body any, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		requestBody = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, requestBody)
	if auth {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	return rr
}

type fakeTokenValidator struct {
	actorID uuid.UUID
}

func (v fakeTokenValidator) ValidateAccessToken(token string) (authout.AccessTokenClaims, error) {
	if token != "test-token" {
		return authout.AccessTokenClaims{}, authout.ErrInvalidAccessToken
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	return authout.AccessTokenClaims{
		UserID:    v.actorID,
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	}, nil
}

type fakeNotificationService struct {
	tokenResult command.TelegramLinkTokenResult
	settings    query.NotificationSettings
	enabled     *bool
	deleted     bool
}

func (s *fakeNotificationService) CreateTelegramLinkToken(_ context.Context, _ command.CreateTelegramLinkCommand) (command.TelegramLinkTokenResult, error) {
	return s.tokenResult, nil
}

func (s *fakeNotificationService) GetNotificationSettings(_ context.Context, _ query.GetNotificationSettingsQuery) (query.NotificationSettings, error) {
	return s.settings, nil
}

func (s *fakeNotificationService) SetTelegramEnabled(_ context.Context, cmd command.SetTelegramEnabledCommand) error {
	s.enabled = &cmd.Enabled
	return nil
}

func (s *fakeNotificationService) DeleteTelegramLink(_ context.Context, _ command.DeleteTelegramLinkCommand) error {
	s.deleted = true
	return nil
}

func (s *fakeNotificationService) HandleTelegramStart(context.Context, command.HandleTelegramStartCommand) error {
	return nil
}

func (s *fakeNotificationService) HandleNotificationRequested(context.Context, command.HandleNotificationRequestedCommand) error {
	return nil
}

func (s *fakeNotificationService) ListNotifications(context.Context, query.ListNotificationsQuery) ([]domain.Notification, error) {
	return nil, nil
}

func (s *fakeNotificationService) CountUnread(context.Context, query.CountUnreadQuery) (int, error) {
	return 0, nil
}

func (s *fakeNotificationService) MarkRead(context.Context, command.MarkReadCommand) error {
	return nil
}

func (s *fakeNotificationService) MarkAllRead(context.Context, command.MarkAllReadCommand) error {
	return nil
}
