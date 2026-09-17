package httpadapter

import (
	"time"

	"go_pet_project/internal/auth/application/command"
	"go_pet_project/internal/platform/httpx"
)

type RegisterResponse struct {
	User UserResponse `json:"user"`
}

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"created_at"`
}

func newRegisterResponse(result command.RegisterResult) RegisterResponse {
	return RegisterResponse{
		User: UserResponse{
			ID:        result.User.ID.String(),
			Email:     result.User.Email,
			Username:  result.User.Username,
			Timezone:  result.User.Timezone,
			CreatedAt: result.User.CreatedAt,
		},
	}
}

type TokenResponse struct {
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

func newLoginResponse(result command.LoginResult) TokenResponse {
	return TokenResponse{
		AccessToken:           result.AccessToken,
		RefreshToken:          result.RefreshToken,
		AccessTokenExpiresAt:  result.AccessTokenExpiresAt,
		RefreshTokenExpiresAt: result.RefreshTokenExpiresAt,
	}
}

func newRefreshResponse(result command.RefreshResult) TokenResponse {
	return TokenResponse{
		AccessToken:           result.AccessToken,
		RefreshToken:          result.RefreshToken,
		AccessTokenExpiresAt:  result.AccessTokenExpiresAt,
		RefreshTokenExpiresAt: result.RefreshTokenExpiresAt,
	}
}

type ErrorResponse = httpx.ErrorResponse
type ErrorBody = httpx.ErrorBody
