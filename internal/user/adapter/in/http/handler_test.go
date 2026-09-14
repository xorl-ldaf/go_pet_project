package httpadapter

import (
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
	"go_pet_project/internal/platform/httpx"
	"go_pet_project/internal/user/application/query"

	"github.com/google/uuid"
)

var errFakeUserService = errors.New("fake user service error")

func TestGetMeSuccess(t *testing.T) {
	userID := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	now := time.Date(2026, 9, 14, 23, 30, 0, 0, time.UTC)
	service := &fakeUserService{
		result: query.GetMeResult{
			ID:        userID,
			Email:     "me@example.com",
			Username:  "me",
			Timezone:  "UTC",
			CreatedAt: now,
			UpdatedAt: now.Add(time.Minute),
		},
	}
	validator := &fakeAccessTokenValidator{
		claims: authout.AccessTokenClaims{UserID: userID},
	}

	recorder := performGetMeRequest(t, service, validator, "Bearer access-token")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.calls != 1 || service.lastQuery.UserID != userID {
		t.Fatalf("service calls=%d query=%#v", service.calls, service.lastQuery)
	}
	assertJSONContentType(t, recorder)
	body := recorder.Body.String()
	if strings.Contains(body, "password") || strings.Contains(body, "password_hash") {
		t.Fatalf("response must not contain password data: %s", body)
	}

	var response GetMeResponse
	decodeResponse(t, recorder, &response)
	if response.ID != userID.String() ||
		response.Email != "me@example.com" ||
		response.Username != "me" ||
		response.Timezone != "UTC" ||
		!response.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestGetMeWithoutToken(t *testing.T) {
	service := &fakeUserService{}
	validator := &fakeAccessTokenValidator{}

	recorder := performGetMeRequest(t, service, validator, "")

	assertErrorResponse(t, recorder, http.StatusUnauthorized, "unauthorized")
	if service.calls != 0 {
		t.Fatalf("service calls = %d, want 0", service.calls)
	}
}

func TestGetMeInvalidToken(t *testing.T) {
	service := &fakeUserService{}
	validator := &fakeAccessTokenValidator{err: authout.ErrInvalidAccessToken}

	recorder := performGetMeRequest(t, service, validator, "Bearer invalid")

	assertErrorResponse(t, recorder, http.StatusUnauthorized, "unauthorized")
	if service.calls != 0 {
		t.Fatalf("service calls = %d, want 0", service.calls)
	}
}

func TestGetMeMissingContextIdentity(t *testing.T) {
	handler := NewHandler(&fakeUserService{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	recorder := httptest.NewRecorder()

	handler.GetMe(recorder, request)

	assertErrorResponse(t, recorder, http.StatusUnauthorized, "unauthorized")
}

func TestGetMeInternalError(t *testing.T) {
	userID := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	service := &fakeUserService{err: errFakeUserService}
	validator := &fakeAccessTokenValidator{
		claims: authout.AccessTokenClaims{UserID: userID},
	}

	recorder := performGetMeRequest(t, service, validator, "Bearer access-token")

	assertErrorResponse(t, recorder, http.StatusInternalServerError, "internal_server_error")
	if strings.Contains(recorder.Body.String(), errFakeUserService.Error()) {
		t.Fatalf("internal error leaked to response")
	}
}

func performGetMeRequest(t *testing.T, service *fakeUserService, validator authhttp.AccessTokenValidator, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	authMiddleware, err := authhttp.NewAuthMiddleware(validator)
	if err != nil {
		t.Fatalf("new auth middleware: %v", err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil))), authMiddleware.Authenticate)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
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

	var response httpx.ErrorResponse
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

type fakeUserService struct {
	result    query.GetMeResult
	err       error
	calls     int
	lastQuery query.GetMeQuery
}

func (s *fakeUserService) GetMe(_ context.Context, q query.GetMeQuery) (query.GetMeResult, error) {
	s.calls++
	s.lastQuery = q
	if s.err != nil {
		return query.GetMeResult{}, s.err
	}

	return s.result, nil
}

type fakeAccessTokenValidator struct {
	claims authout.AccessTokenClaims
	err    error
}

func (v *fakeAccessTokenValidator) ValidateAccessToken(string) (authout.AccessTokenClaims, error) {
	if v.err != nil {
		return authout.AccessTokenClaims{}, v.err
	}

	return v.claims, nil
}
