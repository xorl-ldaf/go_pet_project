package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"go_pet_project/internal/auth/application"
	"go_pet_project/internal/auth/application/command"
	authin "go_pet_project/internal/auth/application/port/in"
	authout "go_pet_project/internal/auth/application/port/out"
	authdomain "go_pet_project/internal/auth/domain"
	userdomain "go_pet_project/internal/user/domain"

	"github.com/google/uuid"
)

var _ authin.AuthService = (*AuthService)(nil)

type AuthService struct {
	users              authout.UserRepository
	passwords          authout.PasswordHasher
	tokens             authout.TokenProvider
	refreshTokens      authout.RefreshTokenRepository
	refreshTokenValues authout.RefreshTokenGenerator
	refreshTokenTTL    time.Duration
	now                func() time.Time
}

func NewAuthService(
	users authout.UserRepository,
	passwords authout.PasswordHasher,
	tokens authout.TokenProvider,
	refreshTokens authout.RefreshTokenRepository,
	refreshTokenValues authout.RefreshTokenGenerator,
	refreshTokenTTL time.Duration,
) (*AuthService, error) {
	if users == nil {
		return nil, errors.New("user repository is required")
	}
	if passwords == nil {
		return nil, errors.New("password hasher is required")
	}
	if tokens == nil {
		return nil, errors.New("token provider is required")
	}
	if refreshTokens == nil {
		return nil, errors.New("refresh token repository is required")
	}
	if refreshTokenValues == nil {
		return nil, errors.New("refresh token generator is required")
	}
	if refreshTokenTTL <= 0 {
		return nil, errors.New("refresh token TTL must be positive")
	}

	return &AuthService{
		users:              users,
		passwords:          passwords,
		tokens:             tokens,
		refreshTokens:      refreshTokens,
		refreshTokenValues: refreshTokenValues,
		refreshTokenTTL:    refreshTokenTTL,
		now:                time.Now,
	}, nil
}

func (s *AuthService) Register(ctx context.Context, cmd command.RegisterCommand) (command.RegisterResult, error) {
	email, username, timezoneName, err := validateRegisterCommand(cmd)
	if err != nil {
		return command.RegisterResult{}, err
	}

	if _, err := s.users.FindByEmail(ctx, email); err == nil {
		return command.RegisterResult{}, application.ErrEmailAlreadyExists
	} else if !errors.Is(err, userdomain.ErrUserNotFound) {
		return command.RegisterResult{}, fmt.Errorf("check user email uniqueness: %w", err)
	}

	if _, err := s.users.FindByUsername(ctx, username); err == nil {
		return command.RegisterResult{}, application.ErrUsernameAlreadyExists
	} else if !errors.Is(err, userdomain.ErrUserNotFound) {
		return command.RegisterResult{}, fmt.Errorf("check username uniqueness: %w", err)
	}

	passwordHash, err := s.passwords.Hash(cmd.Password)
	if err != nil {
		return command.RegisterResult{}, fmt.Errorf("hash password: %w", err)
	}

	now := s.now().UTC()
	user := userdomain.User{
		ID:           uuid.New(),
		Email:        email,
		Username:     username,
		PasswordHash: passwordHash,
		Timezone:     timezoneName,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	created, err := s.users.Create(ctx, user)
	if err != nil {
		return command.RegisterResult{}, fmt.Errorf("create user: %w", err)
	}

	return command.RegisterResult{
		User: toUserResult(created),
	}, nil
}

func (s *AuthService) Login(ctx context.Context, cmd command.LoginCommand) (command.LoginResult, error) {
	email, password, err := validateLoginCommand(cmd)
	if err != nil {
		return command.LoginResult{}, err
	}

	user, err := s.users.FindByEmail(ctx, email)
	if errors.Is(err, userdomain.ErrUserNotFound) {
		return command.LoginResult{}, application.ErrInvalidCredentials
	}
	if err != nil {
		return command.LoginResult{}, fmt.Errorf("find user by email: %w", err)
	}

	if err := s.passwords.Compare(password, user.PasswordHash); err != nil {
		return command.LoginResult{}, application.ErrInvalidCredentials
	}

	accessToken, accessClaims, err := s.tokens.GenerateAccessToken(user.ID)
	if err != nil {
		return command.LoginResult{}, fmt.Errorf("generate access token: %w", err)
	}

	plainRefreshToken, err := s.refreshTokenValues.Generate()
	if err != nil {
		return command.LoginResult{}, fmt.Errorf("generate refresh token: %w", err)
	}

	now := s.now().UTC()
	refreshTokenExpiresAt := now.Add(s.refreshTokenTTL)
	refreshToken := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: s.refreshTokenValues.Hash(plainRefreshToken),
		ExpiresAt: refreshTokenExpiresAt,
		CreatedAt: now,
	}

	if _, err := s.refreshTokens.Create(ctx, refreshToken); err != nil {
		return command.LoginResult{}, fmt.Errorf("create refresh token: %w", err)
	}

	return command.LoginResult{
		UserID:                user.ID,
		AccessToken:           accessToken,
		RefreshToken:          plainRefreshToken,
		AccessTokenExpiresAt:  accessClaims.ExpiresAt,
		RefreshTokenExpiresAt: refreshTokenExpiresAt,
	}, nil
}

func (s *AuthService) Refresh(ctx context.Context, cmd command.RefreshCommand) (command.RefreshResult, error) {
	plainOldRefreshToken := strings.TrimSpace(cmd.RefreshToken)
	if plainOldRefreshToken == "" {
		return command.RefreshResult{}, application.ErrInvalidRefreshToken
	}

	oldTokenHash := s.refreshTokenValues.Hash(plainOldRefreshToken)
	oldToken, err := s.refreshTokens.FindByHash(ctx, oldTokenHash)
	if err != nil {
		if errors.Is(err, authdomain.ErrRefreshTokenNotFound) {
			return command.RefreshResult{}, authdomain.ErrRefreshTokenNotFound
		}
		return command.RefreshResult{}, fmt.Errorf("find refresh token by hash: %w", err)
	}

	now := s.now().UTC()
	if oldToken.IsRevoked() {
		return command.RefreshResult{}, authdomain.ErrRefreshTokenRevoked
	}
	if oldToken.IsExpired(now) {
		return command.RefreshResult{}, authdomain.ErrRefreshTokenExpired
	}

	plainNewRefreshToken, err := s.refreshTokenValues.Generate()
	if err != nil {
		return command.RefreshResult{}, fmt.Errorf("generate refresh token: %w", err)
	}

	newRefreshTokenExpiresAt := now.Add(s.refreshTokenTTL)
	newRefreshToken := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    oldToken.UserID,
		TokenHash: s.refreshTokenValues.Hash(plainNewRefreshToken),
		ExpiresAt: newRefreshTokenExpiresAt,
		CreatedAt: now,
	}

	accessToken, accessClaims, err := s.tokens.GenerateAccessToken(oldToken.UserID)
	if err != nil {
		return command.RefreshResult{}, fmt.Errorf("generate access token: %w", err)
	}

	if _, err := s.refreshTokens.Rotate(ctx, oldTokenHash, newRefreshToken, now); err != nil {
		return command.RefreshResult{}, fmt.Errorf("rotate refresh token: %w", err)
	}

	return command.RefreshResult{
		AccessToken:           accessToken,
		RefreshToken:          plainNewRefreshToken,
		AccessTokenExpiresAt:  accessClaims.ExpiresAt,
		RefreshTokenExpiresAt: newRefreshTokenExpiresAt,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, cmd command.LogoutCommand) (command.LogoutResult, error) {
	plainRefreshToken := strings.TrimSpace(cmd.RefreshToken)
	if plainRefreshToken == "" {
		return command.LogoutResult{}, application.ErrInvalidRefreshToken
	}

	tokenHash := s.refreshTokenValues.Hash(plainRefreshToken)
	token, err := s.refreshTokens.FindByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, authdomain.ErrRefreshTokenNotFound) {
			return command.LogoutResult{}, nil
		}
		return command.LogoutResult{}, fmt.Errorf("find refresh token by hash: %w", err)
	}
	if token.IsRevoked() {
		return command.LogoutResult{}, nil
	}

	if _, err := s.refreshTokens.Revoke(ctx, tokenHash, s.now().UTC()); err != nil {
		if errors.Is(err, authdomain.ErrRefreshTokenNotFound) {
			return command.LogoutResult{}, nil
		}
		return command.LogoutResult{}, fmt.Errorf("revoke refresh token: %w", err)
	}

	return command.LogoutResult{}, nil
}

func validateRegisterCommand(cmd command.RegisterCommand) (string, string, string, error) {
	email, err := validateEmail(cmd.Email)
	if err != nil {
		return "", "", "", err
	}

	username := strings.TrimSpace(cmd.Username)
	if username == "" {
		return "", "", "", application.ErrInvalidUsername
	}

	if cmd.Password == "" {
		return "", "", "", application.ErrInvalidPassword
	}

	timezoneName := strings.TrimSpace(cmd.Timezone)
	if timezoneName == "" {
		return "", "", "", application.ErrInvalidTimezone
	}
	if _, err := time.LoadLocation(timezoneName); err != nil {
		return "", "", "", fmt.Errorf("%w: %s", application.ErrInvalidTimezone, timezoneName)
	}

	return email, username, timezoneName, nil
}

func validateLoginCommand(cmd command.LoginCommand) (string, string, error) {
	email, err := validateEmail(cmd.Email)
	if err != nil {
		return "", "", err
	}

	if cmd.Password == "" {
		return "", "", application.ErrInvalidCredentials
	}

	return email, cmd.Password, nil
}

func validateEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", application.ErrInvalidEmail
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || address.Name != "" {
		return "", application.ErrInvalidEmail
	}

	return email, nil
}

func toUserResult(user userdomain.User) command.UserResult {
	return command.UserResult{
		ID:        user.ID,
		Email:     user.Email,
		Username:  user.Username,
		Timezone:  user.Timezone,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
