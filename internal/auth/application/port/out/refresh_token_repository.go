package out

import (
	"context"
	"time"

	"go_pet_project/internal/auth/domain"
)

type RefreshTokenRepository interface {
	Create(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error)
	FindByHash(ctx context.Context, tokenHash string) (domain.RefreshToken, error)
	Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) (domain.RefreshToken, error)
	Rotate(ctx context.Context, oldTokenHash string, newToken domain.RefreshToken, revokedAt time.Time) (domain.RefreshToken, error)
}
