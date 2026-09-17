package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRefreshTokenState(t *testing.T) {
	now := time.Date(2026, 9, 14, 17, 0, 0, 0, time.UTC)

	active := RefreshToken{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: "hash",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	}
	if active.IsExpired(now) {
		t.Fatalf("future token must not be expired")
	}
	if !active.IsActive(now) {
		t.Fatalf("future non-revoked token must be active")
	}

	expired := active
	expired.ExpiresAt = now.Add(-time.Nanosecond)
	if !expired.IsExpired(now) {
		t.Fatalf("past token must be expired")
	}
	if expired.IsActive(now) {
		t.Fatalf("expired token must not be active")
	}

	revoked := active
	revokedAt := now.Add(-time.Minute)
	revoked.Revoke(revokedAt)
	if !revoked.IsRevoked() {
		t.Fatalf("revoked token must report revoked")
	}
	if revoked.IsActive(now) {
		t.Fatalf("revoked token must not be active")
	}

	secondRevokeAt := now
	revoked.Revoke(secondRevokeAt)
	if !revoked.RevokedAt.Equal(revokedAt) {
		t.Fatalf("second revoke must not overwrite original revoke timestamp")
	}
}
