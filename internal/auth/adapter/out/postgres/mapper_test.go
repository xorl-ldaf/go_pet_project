package postgres

import (
	"testing"
	"time"

	"go_pet_project/internal/auth/domain"

	"github.com/google/uuid"
)

func TestRefreshTokenMapperRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 14, 17, 40, 0, 123456000, time.UTC)
	revokedAt := now.Add(time.Minute)
	token := domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: "stored-token-hash",
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		RevokedAt: &revokedAt,
		CreatedAt: now,
	}

	got := toDomain(toModel(token))
	if got.ID != token.ID ||
		got.UserID != token.UserID ||
		got.TokenHash != token.TokenHash ||
		!got.ExpiresAt.Equal(token.ExpiresAt) ||
		got.RevokedAt == nil ||
		!got.RevokedAt.Equal(*token.RevokedAt) ||
		!got.CreatedAt.Equal(token.CreatedAt) {
		t.Fatalf("round-trip token = %#v, want %#v", got, token)
	}
}
