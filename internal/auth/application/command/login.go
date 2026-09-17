package command

import (
	"time"

	"github.com/google/uuid"
)

type LoginCommand struct {
	Email    string
	Password string
}

type LoginResult struct {
	UserID                uuid.UUID
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
}
