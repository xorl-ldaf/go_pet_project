package integration

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go_pet_project/internal/auth/adapter/out/postgres"
	"go_pet_project/internal/auth/adapter/out/refresh"
	authdomain "go_pet_project/internal/auth/domain"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/migrations"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"

	"github.com/google/uuid"
)

func TestPostgresRefreshTokenRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	startPostgres(ctx, t)
	waitForPostgres(ctx, t, cfg.DB)

	testDBName := "todo_refresh_token_repository_test_" + uuid.NewString()
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

	userRepo := userpostgres.NewRepository(pg.GORM)
	tokenRepo := postgres.NewRefreshTokenRepository(pg.GORM)
	tokenGenerator, err := refresh.NewTokenGenerator(refresh.DefaultTokenBytes)
	if err != nil {
		t.Fatalf("new token generator: %v", err)
	}

	user := newTestUser("refresh-owner@example.com", "refresh_owner")
	createdUser, err := userRepo.Create(ctx, user)
	if err != nil {
		t.Fatalf("create user fixture: %v", err)
	}

	plainToken, err := tokenGenerator.Generate()
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}
	tokenHash := tokenGenerator.Hash(plainToken)
	if tokenHash == plainToken {
		t.Fatalf("refresh token hash must not equal plain token")
	}

	now := time.Date(2026, 9, 14, 18, 0, 0, 123456000, time.UTC)
	refreshToken := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    createdUser.ID,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		CreatedAt: now,
	}

	created, err := tokenRepo.Create(ctx, refreshToken)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	assertRefreshTokensEqual(t, created, refreshToken)

	assertPlainRefreshTokenNotStored(ctx, t, pg.SQL, refreshToken.ID, plainToken, tokenHash)

	found, err := tokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("find refresh token by hash: %v", err)
	}
	assertRefreshTokensEqual(t, found, refreshToken)
	if !found.IsActive(now) {
		t.Fatalf("created refresh token must be active")
	}

	if _, err := tokenRepo.FindByHash(ctx, "missing-token-hash"); !errors.Is(err, authdomain.ErrRefreshTokenNotFound) {
		t.Fatalf("find missing token error = %v, want ErrRefreshTokenNotFound", err)
	}

	duplicate := refreshToken
	duplicate.ID = uuid.New()
	if _, err := tokenRepo.Create(ctx, duplicate); err == nil {
		t.Fatalf("expected duplicate token hash error, got nil")
	}

	revokedAt := now.Add(time.Hour)
	revoked, err := tokenRepo.Revoke(ctx, tokenHash, revokedAt)
	if err != nil {
		t.Fatalf("revoke refresh token: %v", err)
	}
	if revoked.RevokedAt == nil || !revoked.RevokedAt.Equal(revokedAt) {
		t.Fatalf("RevokedAt = %v, want %s", revoked.RevokedAt, revokedAt)
	}
	if !revoked.IsRevoked() {
		t.Fatalf("revoked token must report revoked")
	}

	secondRevokeAt := revokedAt.Add(time.Hour)
	revokedAgain, err := tokenRepo.Revoke(ctx, tokenHash, secondRevokeAt)
	if err != nil {
		t.Fatalf("revoke refresh token again: %v", err)
	}
	if revokedAgain.RevokedAt == nil || !revokedAgain.RevokedAt.Equal(revokedAt) {
		t.Fatalf("second revoke must preserve first revoke time, got %v want %s", revokedAgain.RevokedAt, revokedAt)
	}

	if _, err := tokenRepo.Revoke(ctx, "missing-token-hash", revokedAt); !errors.Is(err, authdomain.ErrRefreshTokenNotFound) {
		t.Fatalf("revoke missing token error = %v, want ErrRefreshTokenNotFound", err)
	}

	rotationOld := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    createdUser.ID,
		TokenHash: tokenGenerator.Hash("rotation-old-token"),
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		CreatedAt: now,
	}
	if _, err := tokenRepo.Create(ctx, rotationOld); err != nil {
		t.Fatalf("create rotation old token: %v", err)
	}

	rotationNew := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    createdUser.ID,
		TokenHash: tokenGenerator.Hash("rotation-new-token"),
		ExpiresAt: now.Add(60 * 24 * time.Hour),
		CreatedAt: now.Add(time.Minute),
	}
	if _, err := tokenRepo.Rotate(ctx, rotationOld.TokenHash, rotationNew, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("rotate refresh token: %v", err)
	}

	rotatedOld, err := tokenRepo.FindByHash(ctx, rotationOld.TokenHash)
	if err != nil {
		t.Fatalf("find rotated old token: %v", err)
	}
	if rotatedOld.RevokedAt == nil {
		t.Fatalf("old refresh token must be revoked after rotation")
	}

	rotatedNew, err := tokenRepo.FindByHash(ctx, rotationNew.TokenHash)
	if err != nil {
		t.Fatalf("find rotated new token: %v", err)
	}
	if rotatedNew.RevokedAt != nil {
		t.Fatalf("new refresh token must be active after rotation")
	}
	if rotatedNew.UserID != rotationOld.UserID {
		t.Fatalf("new refresh token UserID = %s, want %s", rotatedNew.UserID, rotationOld.UserID)
	}

	rollbackOld := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    createdUser.ID,
		TokenHash: tokenGenerator.Hash("rollback-old-token"),
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		CreatedAt: now,
	}
	if _, err := tokenRepo.Create(ctx, rollbackOld); err != nil {
		t.Fatalf("create rollback old token: %v", err)
	}

	invalidNew := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: tokenGenerator.Hash("rollback-invalid-new-token"),
		ExpiresAt: now.Add(60 * 24 * time.Hour),
		CreatedAt: now.Add(time.Minute),
	}
	if _, err := tokenRepo.Rotate(ctx, rollbackOld.TokenHash, invalidNew, now.Add(2*time.Minute)); err == nil {
		t.Fatalf("expected rotate with invalid new token to fail")
	}

	rollbackOldAfterFailure, err := tokenRepo.FindByHash(ctx, rollbackOld.TokenHash)
	if err != nil {
		t.Fatalf("find rollback old token after failed rotation: %v", err)
	}
	if rollbackOldAfterFailure.RevokedAt != nil {
		t.Fatalf("old token revoke must rollback when new token insert fails")
	}

	concurrentOld := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    createdUser.ID,
		TokenHash: tokenGenerator.Hash("concurrent-old-token"),
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		CreatedAt: now,
	}
	if _, err := tokenRepo.Create(ctx, concurrentOld); err != nil {
		t.Fatalf("create concurrent old token: %v", err)
	}

	concurrentB := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    createdUser.ID,
		TokenHash: tokenGenerator.Hash("concurrent-new-token-b"),
		ExpiresAt: now.Add(60 * 24 * time.Hour),
		CreatedAt: now.Add(time.Minute),
	}
	concurrentC := authdomain.RefreshToken{
		ID:        uuid.New(),
		UserID:    createdUser.ID,
		TokenHash: tokenGenerator.Hash("concurrent-new-token-c"),
		ExpiresAt: now.Add(60 * 24 * time.Hour),
		CreatedAt: now.Add(time.Minute),
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := tokenRepo.Rotate(ctx, concurrentOld.TokenHash, concurrentB, now.Add(2*time.Minute))
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		_, err := tokenRepo.Rotate(ctx, concurrentOld.TokenHash, concurrentC, now.Add(2*time.Minute))
		errCh <- err
	}()
	wg.Wait()
	close(errCh)

	successes := 0
	failures := 0
	for err := range errCh {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, authdomain.ErrRefreshTokenRevoked) {
			t.Fatalf("concurrent rotate error = %v, want ErrRefreshTokenRevoked", err)
		}
		failures++
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent rotate successes=%d failures=%d, want 1/1", successes, failures)
	}

	concurrentOldAfterRotation, err := tokenRepo.FindByHash(ctx, concurrentOld.TokenHash)
	if err != nil {
		t.Fatalf("find concurrent old token: %v", err)
	}
	if concurrentOldAfterRotation.RevokedAt == nil {
		t.Fatalf("concurrent old token must be revoked")
	}

	activeDescendants := 0
	if token, err := tokenRepo.FindByHash(ctx, concurrentB.TokenHash); err == nil && token.RevokedAt == nil {
		activeDescendants++
	} else if err != nil && !errors.Is(err, authdomain.ErrRefreshTokenNotFound) {
		t.Fatalf("find concurrent B: %v", err)
	}
	if token, err := tokenRepo.FindByHash(ctx, concurrentC.TokenHash); err == nil && token.RevokedAt == nil {
		activeDescendants++
	} else if err != nil && !errors.Is(err, authdomain.ErrRefreshTokenNotFound) {
		t.Fatalf("find concurrent C: %v", err)
	}
	if activeDescendants != 1 {
		t.Fatalf("active concurrent descendants = %d, want 1", activeDescendants)
	}
}

func assertPlainRefreshTokenNotStored(ctx context.Context, t *testing.T, db *sql.DB, id uuid.UUID, plainToken string, tokenHash string) {
	t.Helper()

	var storedHash string
	if err := db.QueryRowContext(ctx, `SELECT token_hash FROM refresh_tokens WHERE id = $1`, id).Scan(&storedHash); err != nil {
		t.Fatalf("read stored token hash: %v", err)
	}
	if storedHash == plainToken {
		t.Fatalf("plain refresh token must not be stored")
	}
	if storedHash != tokenHash {
		t.Fatalf("stored token hash mismatch")
	}
}

func assertRefreshTokensEqual(t *testing.T, got authdomain.RefreshToken, want authdomain.RefreshToken) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %s, want %s", got.ID, want.ID)
	}
	if got.UserID != want.UserID {
		t.Fatalf("UserID = %s, want %s", got.UserID, want.UserID)
	}
	if got.TokenHash != want.TokenHash {
		t.Fatalf("TokenHash mismatch")
	}
	if !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("ExpiresAt = %s, want %s", got.ExpiresAt, want.ExpiresAt)
	}
	if got.RevokedAt != nil || want.RevokedAt != nil {
		if got.RevokedAt == nil || want.RevokedAt == nil || !got.RevokedAt.Equal(*want.RevokedAt) {
			t.Fatalf("RevokedAt = %v, want %v", got.RevokedAt, want.RevokedAt)
		}
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want %s", got.CreatedAt, want.CreatedAt)
	}
}
