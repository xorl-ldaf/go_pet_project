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

	"go_pet_project/internal/auth/application"
	"go_pet_project/internal/auth/application/command"
	authdomain "go_pet_project/internal/auth/domain"

	"github.com/google/uuid"
)

var errHTTPFakeInternal = errors.New("database unavailable")

func TestRegisterSuccess(t *testing.T) {
	now := time.Date(2026, 9, 14, 22, 0, 0, 0, time.UTC)
	service := &fakeAuthService{
		registerResult: command.RegisterResult{
			User: command.UserResult{
				ID:        uuid.MustParse("10000000-0000-4000-8000-000000000001"),
				Email:     "new@example.com",
				Username:  "new_user",
				Timezone:  "UTC",
				CreatedAt: now,
			},
		},
	}

	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/register", `{
		"email": "new@example.com",
		"username": "new_user",
		"password": "plain-password",
		"timezone": "UTC"
	}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
	assertJSONContentType(t, recorder)
	if service.registerCalls != 1 {
		t.Fatalf("register calls = %d, want 1", service.registerCalls)
	}
	if service.lastRegister.Password != "plain-password" {
		t.Fatalf("handler did not map password into command")
	}
	body := recorder.Body.String()
	if strings.Contains(body, "plain-password") || strings.Contains(body, "password_hash") {
		t.Fatalf("register response must not contain password or hash: %s", body)
	}

	var response RegisterResponse
	decodeResponse(t, recorder, &response)
	if response.User.Email != "new@example.com" || response.User.Username != "new_user" || response.User.Timezone != "UTC" {
		t.Fatalf("unexpected register response: %#v", response)
	}
}

func TestRegisterMalformedJSON(t *testing.T) {
	service := &fakeAuthService{}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/register", `{"email":`)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if service.registerCalls != 0 {
		t.Fatalf("register calls = %d, want 0", service.registerCalls)
	}
}

func TestRegisterUnknownField(t *testing.T) {
	service := &fakeAuthService{}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/register", `{
		"email": "new@example.com",
		"username": "new_user",
		"password": "plain-password",
		"timezone": "UTC",
		"admin": true
	}`)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if service.registerCalls != 0 {
		t.Fatalf("register calls = %d, want 0", service.registerCalls)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	service := &fakeAuthService{registerErr: application.ErrEmailAlreadyExists}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/register", validRegisterBody())

	assertErrorResponse(t, recorder, http.StatusConflict, "email_already_exists")
}

func TestRegisterDuplicateUsername(t *testing.T) {
	service := &fakeAuthService{registerErr: application.ErrUsernameAlreadyExists}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/register", validRegisterBody())

	assertErrorResponse(t, recorder, http.StatusConflict, "username_already_exists")
}

func TestRegisterInternalError(t *testing.T) {
	service := &fakeAuthService{registerErr: errHTTPFakeInternal}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/register", validRegisterBody())

	assertErrorResponse(t, recorder, http.StatusInternalServerError, "internal_server_error")
	if strings.Contains(recorder.Body.String(), "database unavailable") {
		t.Fatalf("internal error response leaked infrastructure details")
	}
}

func TestLoginSuccess(t *testing.T) {
	now := time.Date(2026, 9, 14, 22, 10, 0, 0, time.UTC)
	service := &fakeAuthService{
		loginResult: command.LoginResult{
			AccessToken:           "access-token",
			RefreshToken:          "refresh-token",
			AccessTokenExpiresAt:  now.Add(15 * time.Minute),
			RefreshTokenExpiresAt: now.Add(30 * 24 * time.Hour),
		},
	}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/login", `{"email":"new@example.com","password":"plain-password"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if service.loginCalls != 1 || service.lastLogin.Email != "new@example.com" || service.lastLogin.Password != "plain-password" {
		t.Fatalf("login command was not mapped correctly")
	}

	var response TokenResponse
	decodeResponse(t, recorder, &response)
	if response.AccessToken != "access-token" || response.RefreshToken != "refresh-token" {
		t.Fatalf("unexpected login response: %#v", response)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	service := &fakeAuthService{loginErr: application.ErrInvalidCredentials}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/login", `{"email":"new@example.com","password":"wrong"}`)

	assertErrorResponse(t, recorder, http.StatusUnauthorized, "invalid_credentials")
	if strings.Contains(recorder.Body.String(), "wrong") || strings.Contains(recorder.Body.String(), "email does not exist") {
		t.Fatalf("invalid credentials response leaked detail")
	}
}

func TestRefreshSuccess(t *testing.T) {
	now := time.Date(2026, 9, 14, 22, 20, 0, 0, time.UTC)
	service := &fakeAuthService{
		refreshResult: command.RefreshResult{
			AccessToken:           "new-access-token",
			RefreshToken:          "new-refresh-token",
			AccessTokenExpiresAt:  now.Add(15 * time.Minute),
			RefreshTokenExpiresAt: now.Add(30 * 24 * time.Hour),
		},
	}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"old-refresh-token"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if service.refreshCalls != 1 || service.lastRefresh.RefreshToken != "old-refresh-token" {
		t.Fatalf("refresh command was not mapped correctly")
	}

	var response TokenResponse
	decodeResponse(t, recorder, &response)
	if response.AccessToken != "new-access-token" || response.RefreshToken != "new-refresh-token" {
		t.Fatalf("unexpected refresh response: %#v", response)
	}
}

func TestRefreshInvalidStates(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "expired", err: authdomain.ErrRefreshTokenExpired},
		{name: "revoked", err: authdomain.ErrRefreshTokenRevoked},
		{name: "unknown", err: authdomain.ErrRefreshTokenNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeAuthService{refreshErr: tt.err}
			recorder := performRequest(service, http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"refresh-token"}`)

			assertErrorResponse(t, recorder, http.StatusUnauthorized, "invalid_refresh_token")
		})
	}
}

func TestLogoutSuccess(t *testing.T) {
	service := &fakeAuthService{}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/logout", `{"refresh_token":"refresh-token"}`)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("logout body length = %d, want 0", recorder.Body.Len())
	}
	if service.logoutCalls != 1 || service.lastLogout.RefreshToken != "refresh-token" {
		t.Fatalf("logout command was not mapped correctly")
	}
}

func TestLogoutFailure(t *testing.T) {
	service := &fakeAuthService{logoutErr: errHTTPFakeInternal}
	recorder := performRequest(service, http.MethodPost, "/api/v1/auth/logout", `{"refresh_token":"refresh-token"}`)

	assertErrorResponse(t, recorder, http.StatusInternalServerError, "internal_server_error")
}

func TestWrongMethodDoesNotCallService(t *testing.T) {
	service := &fakeAuthService{}
	recorder := performRequest(service, http.MethodGet, "/api/v1/auth/login", "")

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if service.loginCalls != 0 {
		t.Fatalf("login calls = %d, want 0", service.loginCalls)
	}
}

func validRegisterBody() string {
	return `{"email":"new@example.com","username":"new_user","password":"plain-password","timezone":"UTC"}`
}

func performRequest(service *fakeAuthService, method string, path string, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil))))

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}

	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	return recorder
}

func assertJSONContentType(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	contentType := recorder.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
}

func assertErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, status, recorder.Body.String())
	}
	assertJSONContentType(t, recorder)

	var response ErrorResponse
	decodeResponse(t, recorder, &response)
	if response.Error.Code != code {
		t.Fatalf("error code = %q, want %q", response.Error.Code, code)
	}
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder, dst any) {
	t.Helper()

	if err := json.Unmarshal(recorder.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v: %s", err, recorder.Body.String())
	}
}

type fakeAuthService struct {
	registerResult command.RegisterResult
	registerErr    error
	registerCalls  int
	lastRegister   command.RegisterCommand

	loginResult command.LoginResult
	loginErr    error
	loginCalls  int
	lastLogin   command.LoginCommand

	refreshResult command.RefreshResult
	refreshErr    error
	refreshCalls  int
	lastRefresh   command.RefreshCommand

	logoutResult command.LogoutResult
	logoutErr    error
	logoutCalls  int
	lastLogout   command.LogoutCommand
}

func (s *fakeAuthService) Register(_ context.Context, cmd command.RegisterCommand) (command.RegisterResult, error) {
	s.registerCalls++
	s.lastRegister = cmd
	if s.registerErr != nil {
		return command.RegisterResult{}, s.registerErr
	}

	return s.registerResult, nil
}

func (s *fakeAuthService) Login(_ context.Context, cmd command.LoginCommand) (command.LoginResult, error) {
	s.loginCalls++
	s.lastLogin = cmd
	if s.loginErr != nil {
		return command.LoginResult{}, s.loginErr
	}

	return s.loginResult, nil
}

func (s *fakeAuthService) Refresh(_ context.Context, cmd command.RefreshCommand) (command.RefreshResult, error) {
	s.refreshCalls++
	s.lastRefresh = cmd
	if s.refreshErr != nil {
		return command.RefreshResult{}, s.refreshErr
	}

	return s.refreshResult, nil
}

func (s *fakeAuthService) Logout(_ context.Context, cmd command.LogoutCommand) (command.LogoutResult, error) {
	s.logoutCalls++
	s.lastLogout = cmd
	if s.logoutErr != nil {
		return command.LogoutResult{}, s.logoutErr
	}

	return s.logoutResult, nil
}
