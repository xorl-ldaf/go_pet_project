package out

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidAccessToken = errors.New("invalid access token")
	ErrAccessTokenExpired = errors.New("access token expired")
)

type AccessTokenClaims struct {
	UserID    uuid.UUID
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type TokenProvider interface {
	GenerateAccessToken(userID uuid.UUID) (string, AccessTokenClaims, error)
	ValidateAccessToken(token string) (AccessTokenClaims, error)
}
