package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go_pet_project/internal/auth/application"
	"go_pet_project/internal/auth/application/command"
	authout "go_pet_project/internal/auth/application/port/out"
	authdomain "go_pet_project/internal/auth/domain"
	userdomain "go_pet_project/internal/user/domain"

	"github.com/google/uuid"
)

var errFakeInfrastructure = errors.New("fake infrastructure error")

func TestRegisterSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	users := &fakeUserRepository{
		findByEmailErr:    userdomain.ErrUserNotFound,
		findByUsernameErr: userdomain.ErrUserNotFound,
	}
	passwords := &fakePasswordHasher{hashResult: "hashed-password"}
	service := newTestAuthService(t, users, passwords, nil, nil, nil, now)

	result, err := service.Register(ctx, command.RegisterCommand{
		Email:    "new@example.com",
		Username: "new_user",
		Password: "plain-password",
		Timezone: "Europe/Helsinki",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if len(users.created) != 1 {
		t.Fatalf("created users = %d, want 1", len(users.created))
	}
	created := users.created[0]
	if created.Email != "new@example.com" ||
		created.Username != "new_user" ||
		created.Timezone != "Europe/Helsinki" ||
		created.PasswordHash != "hashed-password" ||
		created.PasswordHash == "plain-password" ||
		created.ID == uuid.Nil ||
		!created.CreatedAt.Equal(now) ||
		!created.UpdatedAt.Equal(now) {
		t.Fatalf("created user has unexpected fields: %#v", created)
	}
	if result.User.ID != created.ID ||
		result.User.Email != created.Email ||
		result.User.Username != created.Username ||
		result.User.Timezone != created.Timezone {
		t.Fatalf("register result does not match created user: %#v", result.User)
	}
	if passwords.hashCalls != 1 {
		t.Fatalf("hash calls = %d, want 1", passwords.hashCalls)
	}
	if passwords.lastPassword != "plain-password" {
		t.Fatalf("password hasher did not receive plaintext password")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	users := &fakeUserRepository{
		findByEmailUser: existingUser(),
	}
	passwords := &fakePasswordHasher{hashResult: "hashed-password"}
	service := newTestAuthService(t, users, passwords, nil, nil, nil, time.Now().UTC())

	_, err := service.Register(context.Background(), validRegisterCommand())
	if !errors.Is(err, application.ErrEmailAlreadyExists) {
		t.Fatalf("register error = %v, want ErrEmailAlreadyExists", err)
	}
	if passwords.hashCalls != 0 {
		t.Fatalf("hash calls = %d, want 0", passwords.hashCalls)
	}
	if len(users.created) != 0 {
		t.Fatalf("create calls = %d, want 0", len(users.created))
	}
}

func TestRegisterDuplicateUsername(t *testing.T) {
	users := &fakeUserRepository{
		findByEmailErr:      userdomain.ErrUserNotFound,
		findByUsernameUser:  existingUser(),
		findByUsernameEmail: "taken_username",
	}
	passwords := &fakePasswordHasher{hashResult: "hashed-password"}
	service := newTestAuthService(t, users, passwords, nil, nil, nil, time.Now().UTC())

	_, err := service.Register(context.Background(), validRegisterCommand())
	if !errors.Is(err, application.ErrUsernameAlreadyExists) {
		t.Fatalf("register error = %v, want ErrUsernameAlreadyExists", err)
	}
	if passwords.hashCalls != 0 {
		t.Fatalf("hash calls = %d, want 0", passwords.hashCalls)
	}
	if len(users.created) != 0 {
		t.Fatalf("create calls = %d, want 0", len(users.created))
	}
}

func TestRegisterHashingFailure(t *testing.T) {
	users := &fakeUserRepository{
		findByEmailErr:    userdomain.ErrUserNotFound,
		findByUsernameErr: userdomain.ErrUserNotFound,
	}
	passwords := &fakePasswordHasher{hashErr: errFakeInfrastructure}
	service := newTestAuthService(t, users, passwords, nil, nil, nil, time.Now().UTC())

	_, err := service.Register(context.Background(), validRegisterCommand())
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("register error = %v, want infrastructure error", err)
	}
	if len(users.created) != 0 {
		t.Fatalf("create calls = %d, want 0", len(users.created))
	}
}

func TestRegisterRepositoryFailure(t *testing.T) {
	users := &fakeUserRepository{
		findByEmailErr:    userdomain.ErrUserNotFound,
		findByUsernameErr: userdomain.ErrUserNotFound,
		createErr:         errFakeInfrastructure,
	}
	passwords := &fakePasswordHasher{hashResult: "hashed-password"}
	service := newTestAuthService(t, users, passwords, nil, nil, nil, time.Now().UTC())

	_, err := service.Register(context.Background(), validRegisterCommand())
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("register error = %v, want infrastructure error", err)
	}
	if len(users.created) != 1 {
		t.Fatalf("create calls = %d, want 1", len(users.created))
	}
}

func TestRegisterInvalidTimezone(t *testing.T) {
	users := &fakeUserRepository{}
	service := newTestAuthService(t, users, &fakePasswordHasher{}, nil, nil, nil, time.Now().UTC())

	cmd := validRegisterCommand()
	cmd.Timezone = "Definitely/NotAZone"

	_, err := service.Register(context.Background(), cmd)
	if !errors.Is(err, application.ErrInvalidTimezone) {
		t.Fatalf("register error = %v, want ErrInvalidTimezone", err)
	}
	if len(users.created) != 0 {
		t.Fatalf("create calls = %d, want 0", len(users.created))
	}
}

func TestLoginSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 19, 15, 0, 0, time.UTC)
	user := existingUser()
	users := &fakeUserRepository{findByEmailUser: user}
	passwords := &fakePasswordHasher{}
	tokens := &fakeTokenProvider{
		accessToken: "access-token",
		claims: authout.AccessTokenClaims{
			UserID:    user.ID,
			IssuedAt:  now,
			ExpiresAt: now.Add(15 * time.Minute),
		},
	}
	refreshValues := &fakeRefreshTokenGenerator{
		plain: "plain-refresh-token",
		hash:  "stored-refresh-token-hash",
	}
	refreshTokens := &fakeRefreshTokenRepository{}
	service := newTestAuthService(t, users, passwords, tokens, refreshTokens, refreshValues, now)

	result, err := service.Login(ctx, command.LoginCommand{
		Email:    user.Email,
		Password: "plain-password",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if result.UserID != user.ID ||
		result.AccessToken != "access-token" ||
		result.RefreshToken != "plain-refresh-token" ||
		!result.AccessTokenExpiresAt.Equal(now.Add(15*time.Minute)) ||
		!result.RefreshTokenExpiresAt.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("unexpected login result: %#v", result)
	}
	if passwords.compareCalls != 1 ||
		passwords.lastPassword != "plain-password" ||
		passwords.lastHash != user.PasswordHash {
		t.Fatalf("password compare was not called correctly")
	}
	if tokens.generateCalls != 1 || tokens.lastUserID != user.ID {
		t.Fatalf("access token provider was not called correctly")
	}
	if refreshValues.generateCalls != 1 || refreshValues.hashCalls != 1 || refreshValues.lastHashedToken != "plain-refresh-token" {
		t.Fatalf("refresh token generator/hash was not called correctly")
	}
	if len(refreshTokens.created) != 1 {
		t.Fatalf("refresh token create calls = %d, want 1", len(refreshTokens.created))
	}
	stored := refreshTokens.created[0]
	if stored.UserID != user.ID ||
		stored.TokenHash != "stored-refresh-token-hash" ||
		stored.TokenHash == "plain-refresh-token" ||
		!stored.ExpiresAt.Equal(now.Add(30*24*time.Hour)) ||
		!stored.CreatedAt.Equal(now) ||
		stored.RevokedAt != nil {
		t.Fatalf("unexpected stored refresh token: %#v", stored)
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	users := &fakeUserRepository{findByEmailErr: userdomain.ErrUserNotFound}
	service := newTestAuthService(t, users, &fakePasswordHasher{}, nil, &fakeRefreshTokenRepository{}, &fakeRefreshTokenGenerator{}, time.Now().UTC())

	_, err := service.Login(context.Background(), command.LoginCommand{Email: "missing@example.com", Password: "password"})
	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	users := &fakeUserRepository{findByEmailUser: existingUser()}
	passwords := &fakePasswordHasher{compareErr: errFakeInfrastructure}
	refreshTokens := &fakeRefreshTokenRepository{}
	service := newTestAuthService(t, users, passwords, nil, refreshTokens, &fakeRefreshTokenGenerator{}, time.Now().UTC())

	_, err := service.Login(context.Background(), command.LoginCommand{Email: "existing@example.com", Password: "wrong-password"})
	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("login error = %v, want ErrInvalidCredentials", err)
	}
	if len(refreshTokens.created) != 0 {
		t.Fatalf("refresh token create calls = %d, want 0", len(refreshTokens.created))
	}
}

func TestLoginRepositoryFailure(t *testing.T) {
	users := &fakeUserRepository{findByEmailErr: errFakeInfrastructure}
	service := newTestAuthService(t, users, &fakePasswordHasher{}, nil, &fakeRefreshTokenRepository{}, &fakeRefreshTokenGenerator{}, time.Now().UTC())

	_, err := service.Login(context.Background(), command.LoginCommand{Email: "existing@example.com", Password: "password"})
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("login error = %v, want infrastructure error", err)
	}
}

func TestLoginTokenGenerationFailure(t *testing.T) {
	users := &fakeUserRepository{findByEmailUser: existingUser()}
	tokens := &fakeTokenProvider{generateErr: errFakeInfrastructure}
	refreshTokens := &fakeRefreshTokenRepository{}
	refreshValues := &fakeRefreshTokenGenerator{plain: "plain-refresh-token", hash: "stored-refresh-token-hash"}
	service := newTestAuthService(t, users, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, time.Now().UTC())

	_, err := service.Login(context.Background(), command.LoginCommand{Email: "existing@example.com", Password: "password"})
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("login error = %v, want token generation error", err)
	}
	if refreshValues.generateCalls != 0 {
		t.Fatalf("refresh token generation calls = %d, want 0", refreshValues.generateCalls)
	}
	if len(refreshTokens.created) != 0 {
		t.Fatalf("refresh token create calls = %d, want 0", len(refreshTokens.created))
	}
}

func TestLoginRefreshPersistenceFailure(t *testing.T) {
	users := &fakeUserRepository{findByEmailUser: existingUser()}
	tokens := &fakeTokenProvider{accessToken: "access-token", claims: authout.AccessTokenClaims{UserID: existingUser().ID, ExpiresAt: time.Now().UTC().Add(time.Minute)}}
	refreshTokens := &fakeRefreshTokenRepository{createErr: errFakeInfrastructure}
	refreshValues := &fakeRefreshTokenGenerator{plain: "plain-refresh-token", hash: "stored-refresh-token-hash"}
	service := newTestAuthService(t, users, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, time.Now().UTC())

	result, err := service.Login(context.Background(), command.LoginCommand{Email: "existing@example.com", Password: "password"})
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("login error = %v, want refresh persistence error", err)
	}
	if result.AccessToken != "" || result.RefreshToken != "" {
		t.Fatalf("login must not return credentials on refresh persistence failure: %#v", result)
	}
}

func TestRefreshSuccess(t *testing.T) {
	now := time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC)
	oldToken := activeRefreshToken(now)
	refreshTokens := &fakeRefreshTokenRepository{findByHashToken: oldToken}
	refreshValues := &fakeRefreshTokenGenerator{
		plain:          "new-plain-refresh-token",
		hash:           "old-token-hash",
		hashByPlain:    map[string]string{"old-plain-refresh-token": "old-token-hash", "new-plain-refresh-token": "new-token-hash"},
		defaultHash:    "fallback-hash",
		useHashByPlain: true,
	}
	tokens := &fakeTokenProvider{
		accessToken: "new-access-token",
		claims: authout.AccessTokenClaims{
			UserID:    oldToken.UserID,
			IssuedAt:  now,
			ExpiresAt: now.Add(15 * time.Minute),
		},
	}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, now)

	result, err := service.Refresh(context.Background(), command.RefreshCommand{RefreshToken: "old-plain-refresh-token"})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if result.AccessToken != "new-access-token" ||
		result.RefreshToken != "new-plain-refresh-token" ||
		!result.AccessTokenExpiresAt.Equal(now.Add(15*time.Minute)) ||
		!result.RefreshTokenExpiresAt.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("unexpected refresh result: %#v", result)
	}
	if refreshTokens.lastFindHash != "old-token-hash" || refreshTokens.lastFindHash == "old-plain-refresh-token" {
		t.Fatalf("FindByHash got %q, want old hash only", refreshTokens.lastFindHash)
	}
	if tokens.generateCalls != 1 || tokens.lastUserID != oldToken.UserID {
		t.Fatalf("access token generated for %s, want %s", tokens.lastUserID, oldToken.UserID)
	}
	if len(refreshTokens.rotations) != 1 {
		t.Fatalf("rotations = %d, want 1", len(refreshTokens.rotations))
	}
	rotation := refreshTokens.rotations[0]
	if rotation.oldHash != "old-token-hash" {
		t.Fatalf("rotation old hash = %q, want old-token-hash", rotation.oldHash)
	}
	if rotation.newToken.UserID != oldToken.UserID {
		t.Fatalf("new refresh token user = %s, want %s", rotation.newToken.UserID, oldToken.UserID)
	}
	if rotation.newToken.TokenHash != "new-token-hash" || rotation.newToken.TokenHash == "new-plain-refresh-token" {
		t.Fatalf("new refresh token stores hash %q", rotation.newToken.TokenHash)
	}
	if rotation.newToken.RevokedAt != nil {
		t.Fatalf("new refresh token must be active")
	}
}

func TestRefreshExpiredToken(t *testing.T) {
	now := time.Date(2026, 9, 14, 20, 10, 0, 0, time.UTC)
	expired := activeRefreshToken(now)
	expired.ExpiresAt = now.Add(-time.Second)
	refreshTokens := &fakeRefreshTokenRepository{findByHashToken: expired}
	refreshValues := &fakeRefreshTokenGenerator{hash: "old-token-hash"}
	tokens := &fakeTokenProvider{accessToken: "access-token"}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, now)

	_, err := service.Refresh(context.Background(), command.RefreshCommand{RefreshToken: "old-plain-refresh-token"})
	if !errors.Is(err, authdomain.ErrRefreshTokenExpired) {
		t.Fatalf("refresh error = %v, want ErrRefreshTokenExpired", err)
	}
	if refreshValues.generateCalls != 0 || tokens.generateCalls != 0 || len(refreshTokens.rotations) != 0 {
		t.Fatalf("expired refresh token must not generate or rotate")
	}
}

func TestRefreshRevokedToken(t *testing.T) {
	now := time.Date(2026, 9, 14, 20, 20, 0, 0, time.UTC)
	revoked := activeRefreshToken(now)
	revokedAt := now.Add(-time.Minute)
	revoked.RevokedAt = &revokedAt
	refreshTokens := &fakeRefreshTokenRepository{findByHashToken: revoked}
	refreshValues := &fakeRefreshTokenGenerator{hash: "old-token-hash"}
	tokens := &fakeTokenProvider{accessToken: "access-token"}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, now)

	_, err := service.Refresh(context.Background(), command.RefreshCommand{RefreshToken: "old-plain-refresh-token"})
	if !errors.Is(err, authdomain.ErrRefreshTokenRevoked) {
		t.Fatalf("refresh error = %v, want ErrRefreshTokenRevoked", err)
	}
	if refreshValues.generateCalls != 0 || tokens.generateCalls != 0 || len(refreshTokens.rotations) != 0 {
		t.Fatalf("revoked refresh token must not generate or rotate")
	}
}

func TestRefreshUnknownToken(t *testing.T) {
	refreshTokens := &fakeRefreshTokenRepository{findByHashErr: authdomain.ErrRefreshTokenNotFound}
	refreshValues := &fakeRefreshTokenGenerator{hash: "old-token-hash"}
	tokens := &fakeTokenProvider{accessToken: "access-token"}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, time.Now().UTC())

	_, err := service.Refresh(context.Background(), command.RefreshCommand{RefreshToken: "old-plain-refresh-token"})
	if !errors.Is(err, authdomain.ErrRefreshTokenNotFound) {
		t.Fatalf("refresh error = %v, want ErrRefreshTokenNotFound", err)
	}
	if refreshValues.generateCalls != 0 || tokens.generateCalls != 0 || len(refreshTokens.rotations) != 0 {
		t.Fatalf("unknown refresh token must not generate or rotate")
	}
}

func TestRefreshRepositoryFailure(t *testing.T) {
	refreshTokens := &fakeRefreshTokenRepository{findByHashErr: errFakeInfrastructure}
	refreshValues := &fakeRefreshTokenGenerator{hash: "old-token-hash"}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, nil, refreshTokens, refreshValues, time.Now().UTC())

	_, err := service.Refresh(context.Background(), command.RefreshCommand{RefreshToken: "old-plain-refresh-token"})
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("refresh error = %v, want infrastructure error", err)
	}
}

func TestRefreshTokenGenerationFailure(t *testing.T) {
	now := time.Date(2026, 9, 14, 20, 30, 0, 0, time.UTC)
	refreshTokens := &fakeRefreshTokenRepository{findByHashToken: activeRefreshToken(now)}
	refreshValues := &fakeRefreshTokenGenerator{hash: "old-token-hash", generateErr: errFakeInfrastructure}
	tokens := &fakeTokenProvider{accessToken: "access-token"}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, now)

	_, err := service.Refresh(context.Background(), command.RefreshCommand{RefreshToken: "old-plain-refresh-token"})
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("refresh error = %v, want generation error", err)
	}
	if tokens.generateCalls != 0 || len(refreshTokens.rotations) != 0 {
		t.Fatalf("generation failure must not create access token or rotate")
	}
}

func TestRefreshAccessTokenGenerationFailure(t *testing.T) {
	now := time.Date(2026, 9, 14, 20, 40, 0, 0, time.UTC)
	refreshTokens := &fakeRefreshTokenRepository{findByHashToken: activeRefreshToken(now)}
	refreshValues := &fakeRefreshTokenGenerator{
		plain:          "new-plain-refresh-token",
		hashByPlain:    map[string]string{"old-plain-refresh-token": "old-token-hash", "new-plain-refresh-token": "new-token-hash"},
		useHashByPlain: true,
	}
	tokens := &fakeTokenProvider{generateErr: errFakeInfrastructure}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, now)

	_, err := service.Refresh(context.Background(), command.RefreshCommand{RefreshToken: "old-plain-refresh-token"})
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("refresh error = %v, want access token generation error", err)
	}
	if len(refreshTokens.rotations) != 0 {
		t.Fatalf("access token generation failure must not rotate")
	}
}

func TestRefreshRotationFailure(t *testing.T) {
	now := time.Date(2026, 9, 14, 20, 50, 0, 0, time.UTC)
	refreshTokens := &fakeRefreshTokenRepository{
		findByHashToken: activeRefreshToken(now),
		rotateErr:       errFakeInfrastructure,
	}
	refreshValues := &fakeRefreshTokenGenerator{
		plain:          "new-plain-refresh-token",
		hashByPlain:    map[string]string{"old-plain-refresh-token": "old-token-hash", "new-plain-refresh-token": "new-token-hash"},
		useHashByPlain: true,
	}
	tokens := &fakeTokenProvider{accessToken: "access-token", claims: authout.AccessTokenClaims{ExpiresAt: now.Add(time.Minute)}}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, tokens, refreshTokens, refreshValues, now)

	result, err := service.Refresh(context.Background(), command.RefreshCommand{RefreshToken: "old-plain-refresh-token"})
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("refresh error = %v, want rotation error", err)
	}
	if result.AccessToken != "" || result.RefreshToken != "" {
		t.Fatalf("refresh must not return successful credentials on rotation failure: %#v", result)
	}
}

func TestLogoutSuccess(t *testing.T) {
	now := time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC)
	refreshTokens := &fakeRefreshTokenRepository{findByHashToken: activeRefreshToken(now)}
	refreshValues := &fakeRefreshTokenGenerator{hash: "token-hash"}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, nil, refreshTokens, refreshValues, now)

	if _, err := service.Logout(context.Background(), command.LogoutCommand{RefreshToken: "plain-refresh-token"}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if refreshTokens.lastFindHash != "token-hash" || refreshTokens.lastFindHash == "plain-refresh-token" {
		t.Fatalf("FindByHash got %q, want hash only", refreshTokens.lastFindHash)
	}
	if len(refreshTokens.revokes) != 1 || refreshTokens.revokes[0].tokenHash != "token-hash" {
		t.Fatalf("revoke calls = %#v, want token-hash", refreshTokens.revokes)
	}
}

func TestLogoutRepeatedAlreadyRevoked(t *testing.T) {
	now := time.Date(2026, 9, 14, 21, 10, 0, 0, time.UTC)
	revoked := activeRefreshToken(now)
	revokedAt := now.Add(-time.Minute)
	revoked.RevokedAt = &revokedAt
	refreshTokens := &fakeRefreshTokenRepository{findByHashToken: revoked}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, nil, refreshTokens, &fakeRefreshTokenGenerator{hash: "token-hash"}, now)

	if _, err := service.Logout(context.Background(), command.LogoutCommand{RefreshToken: "plain-refresh-token"}); err != nil {
		t.Fatalf("logout already revoked: %v", err)
	}
	if len(refreshTokens.revokes) != 0 {
		t.Fatalf("already revoked logout must not call revoke again")
	}
}

func TestLogoutUnknownToken(t *testing.T) {
	refreshTokens := &fakeRefreshTokenRepository{findByHashErr: authdomain.ErrRefreshTokenNotFound}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, nil, refreshTokens, &fakeRefreshTokenGenerator{hash: "token-hash"}, time.Now().UTC())

	if _, err := service.Logout(context.Background(), command.LogoutCommand{RefreshToken: "plain-refresh-token"}); err != nil {
		t.Fatalf("logout unknown token must be success, got %v", err)
	}
	if len(refreshTokens.revokes) != 0 {
		t.Fatalf("unknown logout must not call revoke")
	}
}

func TestLogoutRepositoryFailure(t *testing.T) {
	refreshTokens := &fakeRefreshTokenRepository{findByHashErr: errFakeInfrastructure}
	service := newTestAuthService(t, &fakeUserRepository{}, &fakePasswordHasher{}, nil, refreshTokens, &fakeRefreshTokenGenerator{hash: "token-hash"}, time.Now().UTC())

	_, err := service.Logout(context.Background(), command.LogoutCommand{RefreshToken: "plain-refresh-token"})
	if !errors.Is(err, errFakeInfrastructure) {
		t.Fatalf("logout error = %v, want infrastructure error", err)
	}
}

func validRegisterCommand() command.RegisterCommand {
	return command.RegisterCommand{
		Email:    "new@example.com",
		Username: "new_user",
		Password: "plain-password",
		Timezone: "UTC",
	}
}

func activeRefreshToken(now time.Time) authdomain.RefreshToken {
	return authdomain.RefreshToken{
		ID:        uuid.MustParse("20000000-0000-4000-8000-000000000001"),
		UserID:    uuid.MustParse("10000000-0000-4000-8000-000000000001"),
		TokenHash: "old-token-hash",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now.Add(-time.Hour),
	}
}

func existingUser() userdomain.User {
	now := time.Date(2026, 9, 14, 19, 30, 0, 0, time.UTC)

	return userdomain.User{
		ID:           uuid.MustParse("10000000-0000-4000-8000-000000000001"),
		Email:        "existing@example.com",
		Username:     "existing",
		PasswordHash: "stored-password-hash",
		Timezone:     "UTC",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func newTestAuthService(
	t *testing.T,
	users *fakeUserRepository,
	passwords *fakePasswordHasher,
	tokens *fakeTokenProvider,
	refreshTokens *fakeRefreshTokenRepository,
	refreshValues *fakeRefreshTokenGenerator,
	now time.Time,
) *AuthService {
	t.Helper()

	if tokens == nil {
		tokens = &fakeTokenProvider{
			accessToken: "access-token",
			claims: authout.AccessTokenClaims{
				UserID:    existingUser().ID,
				IssuedAt:  now,
				ExpiresAt: now.Add(15 * time.Minute),
			},
		}
	}
	if refreshTokens == nil {
		refreshTokens = &fakeRefreshTokenRepository{}
	}
	if refreshValues == nil {
		refreshValues = &fakeRefreshTokenGenerator{
			plain: "plain-refresh-token",
			hash:  "stored-refresh-token-hash",
		}
	}

	service, err := NewAuthService(users, passwords, tokens, refreshTokens, refreshValues, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	service.now = func() time.Time {
		return now
	}

	return service
}

type fakeUserRepository struct {
	findByEmailUser     userdomain.User
	findByEmailErr      error
	findByUsernameUser  userdomain.User
	findByUsernameEmail string
	findByUsernameErr   error
	createErr           error
	created             []userdomain.User
}

func (r *fakeUserRepository) Create(_ context.Context, user userdomain.User) (userdomain.User, error) {
	r.created = append(r.created, user)
	if r.createErr != nil {
		return userdomain.User{}, r.createErr
	}

	return user, nil
}

func (r *fakeUserRepository) FindByEmail(_ context.Context, _ string) (userdomain.User, error) {
	if r.findByEmailErr != nil {
		return userdomain.User{}, r.findByEmailErr
	}

	return r.findByEmailUser, nil
}

func (r *fakeUserRepository) FindByUsername(_ context.Context, _ string) (userdomain.User, error) {
	if r.findByUsernameErr != nil {
		return userdomain.User{}, r.findByUsernameErr
	}

	return r.findByUsernameUser, nil
}

type fakePasswordHasher struct {
	hashResult   string
	hashErr      error
	compareErr   error
	hashCalls    int
	compareCalls int
	lastPassword string
	lastHash     string
}

func (h *fakePasswordHasher) Hash(password string) (string, error) {
	h.hashCalls++
	h.lastPassword = password
	if h.hashErr != nil {
		return "", h.hashErr
	}

	return h.hashResult, nil
}

func (h *fakePasswordHasher) Compare(password string, encodedHash string) error {
	h.compareCalls++
	h.lastPassword = password
	h.lastHash = encodedHash

	return h.compareErr
}

type fakeTokenProvider struct {
	accessToken   string
	claims        authout.AccessTokenClaims
	generateErr   error
	generateCalls int
	lastUserID    uuid.UUID
}

func (p *fakeTokenProvider) GenerateAccessToken(userID uuid.UUID) (string, authout.AccessTokenClaims, error) {
	p.generateCalls++
	p.lastUserID = userID
	if p.generateErr != nil {
		return "", authout.AccessTokenClaims{}, p.generateErr
	}

	claims := p.claims
	claims.UserID = userID

	return p.accessToken, claims, nil
}

func (p *fakeTokenProvider) ValidateAccessToken(_ string) (authout.AccessTokenClaims, error) {
	return p.claims, nil
}

type fakeRefreshTokenGenerator struct {
	plain           string
	hash            string
	hashByPlain     map[string]string
	defaultHash     string
	useHashByPlain  bool
	generateErr     error
	generateCalls   int
	hashCalls       int
	lastHashedToken string
}

func (g *fakeRefreshTokenGenerator) Generate() (string, error) {
	g.generateCalls++
	if g.generateErr != nil {
		return "", g.generateErr
	}

	return g.plain, nil
}

func (g *fakeRefreshTokenGenerator) Hash(token string) string {
	g.hashCalls++
	g.lastHashedToken = token
	if g.useHashByPlain {
		if hash, ok := g.hashByPlain[token]; ok {
			return hash
		}
		return g.defaultHash
	}

	return g.hash
}

type fakeRefreshTokenRepository struct {
	createErr       error
	findByHashToken authdomain.RefreshToken
	findByHashErr   error
	rotateErr       error
	revokeErr       error
	lastFindHash    string
	created         []authdomain.RefreshToken
	rotations       []fakeRotation
	revokes         []fakeRevoke
}

type fakeRotation struct {
	oldHash   string
	newToken  authdomain.RefreshToken
	revokedAt time.Time
}

type fakeRevoke struct {
	tokenHash string
	revokedAt time.Time
}

func (r *fakeRefreshTokenRepository) Create(_ context.Context, token authdomain.RefreshToken) (authdomain.RefreshToken, error) {
	r.created = append(r.created, token)
	if r.createErr != nil {
		return authdomain.RefreshToken{}, r.createErr
	}

	return token, nil
}

func (r *fakeRefreshTokenRepository) FindByHash(_ context.Context, tokenHash string) (authdomain.RefreshToken, error) {
	r.lastFindHash = tokenHash
	if r.findByHashErr != nil {
		return authdomain.RefreshToken{}, r.findByHashErr
	}
	if r.findByHashToken.ID == uuid.Nil {
		return authdomain.RefreshToken{}, authdomain.ErrRefreshTokenNotFound
	}

	return r.findByHashToken, nil
}

func (r *fakeRefreshTokenRepository) Revoke(_ context.Context, tokenHash string, revokedAt time.Time) (authdomain.RefreshToken, error) {
	r.revokes = append(r.revokes, fakeRevoke{tokenHash: tokenHash, revokedAt: revokedAt})
	if r.revokeErr != nil {
		return authdomain.RefreshToken{}, r.revokeErr
	}

	token := r.findByHashToken
	token.Revoke(revokedAt)
	return token, nil
}

func (r *fakeRefreshTokenRepository) Rotate(_ context.Context, oldTokenHash string, newToken authdomain.RefreshToken, revokedAt time.Time) (authdomain.RefreshToken, error) {
	r.rotations = append(r.rotations, fakeRotation{oldHash: oldTokenHash, newToken: newToken, revokedAt: revokedAt})
	if r.rotateErr != nil {
		return authdomain.RefreshToken{}, r.rotateErr
	}

	return newToken, nil
}
