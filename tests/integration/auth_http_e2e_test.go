package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	authjwt "go_pet_project/internal/auth/adapter/out/jwt"
	authpassword "go_pet_project/internal/auth/adapter/out/password"
	authpostgres "go_pet_project/internal/auth/adapter/out/postgres"
	authrefresh "go_pet_project/internal/auth/adapter/out/refresh"
	authservice "go_pet_project/internal/auth/application/service"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/migrations"
	userhttp "go_pet_project/internal/user/adapter/in/http"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"
	userservice "go_pet_project/internal/user/application/service"

	"github.com/google/uuid"
)

func TestAuthHTTPE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	startPostgres(ctx, t)
	waitForPostgres(ctx, t, cfg.DB)

	testDBName := "todo_auth_http_e2e_test_" + uuid.NewString()
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
	defer pg.Close()

	router := buildAuthRouter(t, pg)

	email := "e2e@example.com"
	username := "e2e_user"
	password := "plain-password"

	register := requestJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]any{
		"email":    email,
		"username": username,
		"password": password,
		"timezone": "Europe/Helsinki",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201: %s", register.Code, register.Body.String())
	}
	assertRegisteredUserPersisted(ctx, t, pg.SQL, email, username, password)

	login := requestJSON(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": password,
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200: %s", login.Code, login.Body.String())
	}
	loginTokens := decodeTokenResponse(t, login)
	if loginTokens.AccessToken == "" || loginTokens.RefreshToken == "" {
		t.Fatalf("login response must contain access and refresh token")
	}
	assertPlainRefreshTokenNotInDatabase(ctx, t, pg.SQL, loginTokens.RefreshToken)

	meWithoutToken := request(t, router, http.MethodGet, "/api/v1/users/me", nil, nil)
	if meWithoutToken.Code != http.StatusUnauthorized {
		t.Fatalf("users/me without token status = %d, want 401: %s", meWithoutToken.Code, meWithoutToken.Body.String())
	}

	meBadToken := request(t, router, http.MethodGet, "/api/v1/users/me", nil, map[string]string{
		"Authorization": "Bearer " + corruptToken(loginTokens.AccessToken),
	})
	if meBadToken.Code != http.StatusUnauthorized {
		t.Fatalf("users/me bad token status = %d, want 401: %s", meBadToken.Code, meBadToken.Body.String())
	}

	me := request(t, router, http.MethodGet, "/api/v1/users/me", nil, map[string]string{
		"Authorization": "Bearer " + loginTokens.AccessToken,
	})
	if me.Code != http.StatusOK {
		t.Fatalf("users/me status = %d, want 200: %s", me.Code, me.Body.String())
	}
	meResponse := decodeGetMeResponse(t, me)
	if meResponse.Email != email || meResponse.Username != username || meResponse.Timezone != "Europe/Helsinki" {
		t.Fatalf("users/me response mismatch: %#v", meResponse)
	}
	if bytes.Contains(me.Body.Bytes(), []byte("password_hash")) || bytes.Contains(me.Body.Bytes(), []byte(password)) {
		t.Fatalf("users/me response leaked password data: %s", me.Body.String())
	}

	refresh := requestJSON(t, router, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": loginTokens.RefreshToken,
	})
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want 200: %s", refresh.Code, refresh.Body.String())
	}
	refreshedTokens := decodeTokenResponse(t, refresh)
	if refreshedTokens.AccessToken == "" || refreshedTokens.RefreshToken == "" || refreshedTokens.RefreshToken == loginTokens.RefreshToken {
		t.Fatalf("refresh response must contain a new token pair")
	}

	meWithRefreshedAccess := request(t, router, http.MethodGet, "/api/v1/users/me", nil, map[string]string{
		"Authorization": "Bearer " + refreshedTokens.AccessToken,
	})
	if meWithRefreshedAccess.Code != http.StatusOK {
		t.Fatalf("users/me with refreshed access status = %d, want 200: %s", meWithRefreshedAccess.Code, meWithRefreshedAccess.Body.String())
	}

	reuseOld := requestJSON(t, router, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": loginTokens.RefreshToken,
	})
	if reuseOld.Code != http.StatusUnauthorized {
		t.Fatalf("old refresh reuse status = %d, want 401", reuseOld.Code)
	}

	logout := requestJSON(t, router, http.MethodPost, "/api/v1/auth/logout", map[string]any{
		"refresh_token": refreshedTokens.RefreshToken,
	})
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204: %s", logout.Code, logout.Body.String())
	}
	if logout.Body.Len() != 0 {
		t.Fatalf("logout response body length = %d, want 0", logout.Body.Len())
	}

	reuseLoggedOut := requestJSON(t, router, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": refreshedTokens.RefreshToken,
	})
	if reuseLoggedOut.Code != http.StatusUnauthorized {
		t.Fatalf("logged out refresh reuse status = %d, want 401", reuseLoggedOut.Code)
	}
}

func buildAuthRouter(t *testing.T, pg *database.Postgres) http.Handler {
	t.Helper()

	passwordHasher, err := authpassword.NewHasher(4)
	if err != nil {
		t.Fatalf("new password hasher: %v", err)
	}
	tokenProvider, err := authjwt.NewProvider("e2e-jwt-secret-with-at-least-32-characters", 15*time.Minute)
	if err != nil {
		t.Fatalf("new token provider: %v", err)
	}
	refreshTokenGenerator, err := authrefresh.NewTokenGenerator(authrefresh.DefaultTokenBytes)
	if err != nil {
		t.Fatalf("new refresh token generator: %v", err)
	}

	authService, err := authservice.NewAuthService(
		userpostgres.NewRepository(pg.GORM),
		passwordHasher,
		tokenProvider,
		authpostgres.NewRefreshTokenRepository(pg.GORM),
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
	userService, err := userservice.NewUserService(userpostgres.NewRepository(pg.GORM))
	if err != nil {
		t.Fatalf("new user service: %v", err)
	}

	mux := http.NewServeMux()
	authhttp.RegisterRoutes(mux, authhttp.NewHandler(authService, slog.New(slog.NewTextHandler(io.Discard, nil))))
	userhttp.RegisterRoutes(mux, userhttp.NewHandler(userService, slog.New(slog.NewTextHandler(io.Discard, nil))), authMiddleware.Authenticate)

	return mux
}

func requestJSON(t *testing.T, handler http.Handler, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	return request(t, handler, method, path, payload, map[string]string{"Content-Type": "application/json"})
}

func request(t *testing.T, handler http.Handler, method string, path string, payload []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	request := httptest.NewRequest(method, path, body)
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

func decodeTokenResponse(t *testing.T, recorder *httptest.ResponseRecorder) authhttp.TokenResponse {
	t.Helper()

	var response authhttp.TokenResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode token response: %v: %s", err, recorder.Body.String())
	}

	return response
}

func decodeGetMeResponse(t *testing.T, recorder *httptest.ResponseRecorder) userhttp.GetMeResponse {
	t.Helper()

	var response userhttp.GetMeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode users/me response: %v: %s", err, recorder.Body.String())
	}

	return response
}

func corruptToken(token string) string {
	signatureStart := strings.LastIndex(token, ".") + 1
	if token == "" || signatureStart <= 0 || signatureStart >= len(token) {
		return "invalid"
	}

	original := token[signatureStart]
	replacement := byte('a')
	if original == replacement {
		replacement = 'b'
	}

	return token[:signatureStart] + string(replacement) + token[signatureStart+1:]
}

func assertRegisteredUserPersisted(ctx context.Context, t *testing.T, db *sql.DB, email string, username string, plainPassword string) {
	t.Helper()

	var storedEmail, storedUsername, timezone, passwordHash string
	if err := db.QueryRowContext(ctx, `
		SELECT email, username, timezone, password_hash
		FROM users
		WHERE email = $1
	`, email).Scan(&storedEmail, &storedUsername, &timezone, &passwordHash); err != nil {
		t.Fatalf("read registered user: %v", err)
	}
	if storedEmail != email || storedUsername != username || timezone != "Europe/Helsinki" {
		t.Fatalf("registered user persistence mismatch")
	}
	if passwordHash == "" || passwordHash == plainPassword {
		t.Fatalf("password hash must be stored and must not equal plaintext")
	}
}

func assertPlainRefreshTokenNotInDatabase(ctx context.Context, t *testing.T, db *sql.DB, plainRefreshToken string) {
	t.Helper()

	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM refresh_tokens
		WHERE token_hash = $1
	`, plainRefreshToken).Scan(&count); err != nil {
		t.Fatalf("check plain refresh token storage: %v", err)
	}
	if count != 0 {
		t.Fatalf("plain refresh token must not be stored in token_hash")
	}
}
