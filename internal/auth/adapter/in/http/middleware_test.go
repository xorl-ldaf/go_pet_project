package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authout "go_pet_project/internal/auth/application/port/out"

	"github.com/google/uuid"
)

func TestAuthMiddlewareValidTokenCallsNextWithUserID(t *testing.T) {
	userID := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	validator := &fakeAccessTokenValidator{
		claims: authout.AccessTokenClaims{UserID: userID},
	}
	middleware := newTestAuthMiddleware(t, validator)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true

		got, ok := UserIDFromContext(r.Context())
		if !ok {
			t.Fatalf("UserIDFromContext ok = false, want true")
		}
		if got != userID {
			t.Fatalf("user id = %s, want %s", got, userID)
		}

		w.WriteHeader(http.StatusAccepted)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	recorder := httptest.NewRecorder()

	middleware.Authenticate(next).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", recorder.Code)
	}
	if !nextCalled {
		t.Fatalf("next handler was not called")
	}
	if validator.calls != 1 || validator.lastToken != "access-token" {
		t.Fatalf("validator got calls=%d token=%q", validator.calls, validator.lastToken)
	}
}

func TestAuthMiddlewareRejectsMissingHeader(t *testing.T) {
	assertAuthMiddlewareRejects(t, "", nil)
}

func TestAuthMiddlewareRejectsMalformedHeader(t *testing.T) {
	assertAuthMiddlewareRejects(t, "Bearer access-token extra", nil)
}

func TestAuthMiddlewareRejectsWrongScheme(t *testing.T) {
	assertAuthMiddlewareRejects(t, "Basic access-token", nil)
}

func TestAuthMiddlewareRejectsEmptyBearer(t *testing.T) {
	assertAuthMiddlewareRejects(t, "Bearer", nil)
}

func TestAuthMiddlewareRejectsInvalidToken(t *testing.T) {
	assertAuthMiddlewareRejects(t, "Bearer invalid", authout.ErrInvalidAccessToken)
}

func TestAuthMiddlewareRejectsExpiredToken(t *testing.T) {
	assertAuthMiddlewareRejects(t, "Bearer expired", authout.ErrAccessTokenExpired)
}

func TestUserIDFromContext(t *testing.T) {
	userID := uuid.MustParse("10000000-0000-4000-8000-000000000002")

	got, ok := UserIDFromContext(contextWithUserID(context.Background(), userID))
	if !ok {
		t.Fatalf("UserIDFromContext ok = false, want true")
	}
	if got != userID {
		t.Fatalf("user id = %s, want %s", got, userID)
	}

	if got, ok := UserIDFromContext(context.Background()); ok || got != uuid.Nil {
		t.Fatalf("absent user id = %s, %v; want nil false", got, ok)
	}
}

func assertAuthMiddlewareRejects(t *testing.T, authorization string, validateErr error) {
	t.Helper()

	validator := &fakeAccessTokenValidator{
		claims: authout.AccessTokenClaims{
			UserID:    uuid.MustParse("10000000-0000-4000-8000-000000000001"),
			IssuedAt:  time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(time.Minute),
		},
		err: validateErr,
	}
	middleware := newTestAuthMiddleware(t, validator)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	recorder := httptest.NewRecorder()

	middleware.Authenticate(next).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", recorder.Code, recorder.Body.String())
	}
	if nextCalled {
		t.Fatalf("next handler must not be called")
	}
	if validateErr == nil && validator.calls != 0 {
		t.Fatalf("validator calls = %d, want 0", validator.calls)
	}
	if validateErr != nil && validator.calls != 1 {
		t.Fatalf("validator calls = %d, want 1", validator.calls)
	}

	var response ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != "unauthorized" {
		t.Fatalf("error code = %q, want unauthorized", response.Error.Code)
	}
	if errors.Is(validateErr, authout.ErrAccessTokenExpired) && recorder.Body.String() == authout.ErrAccessTokenExpired.Error() {
		t.Fatalf("raw token error leaked to response")
	}
}

func newTestAuthMiddleware(t *testing.T, validator AccessTokenValidator) *AuthMiddleware {
	t.Helper()

	middleware, err := NewAuthMiddleware(validator)
	if err != nil {
		t.Fatalf("new auth middleware: %v", err)
	}

	return middleware
}

type fakeAccessTokenValidator struct {
	claims    authout.AccessTokenClaims
	err       error
	calls     int
	lastToken string
}

func (v *fakeAccessTokenValidator) ValidateAccessToken(token string) (authout.AccessTokenClaims, error) {
	v.calls++
	v.lastToken = token
	if v.err != nil {
		return authout.AccessTokenClaims{}, v.err
	}

	return v.claims, nil
}
