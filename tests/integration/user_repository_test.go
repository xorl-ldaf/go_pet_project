package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/migrations"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"
	"go_pet_project/internal/user/domain"

	"github.com/google/uuid"
)

func TestPostgresUserRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	startPostgres(ctx, t)
	waitForPostgres(ctx, t, cfg.DB)

	testDBName := "todo_user_repository_test_" + uuid.NewString()
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

	repo := userpostgres.NewRepository(pg.GORM)

	t.Run("create and find user", func(t *testing.T) {
		user := newTestUser("repository-create@example.com", "repository_create")

		created, err := repo.Create(ctx, user)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		assertUsersEqual(t, created, user)

		byID, err := repo.FindByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("find by id: %v", err)
		}
		assertUsersEqual(t, byID, user)

		byEmail, err := repo.FindByEmail(ctx, user.Email)
		if err != nil {
			t.Fatalf("find by email: %v", err)
		}
		assertUsersEqual(t, byEmail, user)

		byUsername, err := repo.FindByUsername(ctx, user.Username)
		if err != nil {
			t.Fatalf("find by username: %v", err)
		}
		assertUsersEqual(t, byUsername, user)
	})

	t.Run("not found is domain error", func(t *testing.T) {
		if _, err := repo.FindByID(ctx, uuid.New()); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("find by unknown id error = %v, want ErrUserNotFound", err)
		}
		if _, err := repo.FindByEmail(ctx, "missing@example.com"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("find by unknown email error = %v, want ErrUserNotFound", err)
		}
		if _, err := repo.FindByUsername(ctx, "missing_username"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("find by unknown username error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("find by ids", func(t *testing.T) {
		empty, err := repo.FindByIDs(ctx, nil)
		if err != nil {
			t.Fatalf("find by empty ids: %v", err)
		}
		if len(empty) != 0 {
			t.Fatalf("empty FindByIDs result length = %d, want 0", len(empty))
		}

		first := newTestUser("find-by-ids-a@example.com", "find_by_ids_a")
		second := newTestUser("find-by-ids-b@example.com", "find_by_ids_b")
		if _, err := repo.Create(ctx, first); err != nil {
			t.Fatalf("create first user: %v", err)
		}
		if _, err := repo.Create(ctx, second); err != nil {
			t.Fatalf("create second user: %v", err)
		}

		found, err := repo.FindByIDs(ctx, []uuid.UUID{second.ID, first.ID, uuid.New()})
		if err != nil {
			t.Fatalf("find by ids: %v", err)
		}
		if len(found) != 2 {
			t.Fatalf("FindByIDs result length = %d, want 2", len(found))
		}
		assertUsersEqual(t, found[0], first)
		assertUsersEqual(t, found[1], second)
	})

	t.Run("duplicate email returns error", func(t *testing.T) {
		first := newTestUser("duplicate-email@example.com", "duplicate_email_a")
		second := newTestUser("duplicate-email@example.com", "duplicate_email_b")

		if _, err := repo.Create(ctx, first); err != nil {
			t.Fatalf("create first user: %v", err)
		}
		if _, err := repo.Create(ctx, second); err == nil {
			t.Fatalf("expected duplicate email error, got nil")
		}
	})

	t.Run("duplicate username returns error", func(t *testing.T) {
		first := newTestUser("duplicate-username-a@example.com", "duplicate_username")
		second := newTestUser("duplicate-username-b@example.com", "duplicate_username")

		if _, err := repo.Create(ctx, first); err != nil {
			t.Fatalf("create first user: %v", err)
		}
		if _, err := repo.Create(ctx, second); err == nil {
			t.Fatalf("expected duplicate username error, got nil")
		}
	})

	t.Run("uses caller context", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.FindByEmail(canceledCtx, "repository-create@example.com")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("find with canceled context error = %v, want context.Canceled", err)
		}
	})
}

func newTestUser(email string, username string) domain.User {
	now := time.Date(2026, 9, 14, 16, 45, 0, 123456000, time.UTC)

	return domain.User{
		ID:           uuid.New(),
		Email:        email,
		Username:     username,
		PasswordHash: "$fake-hash-for-test",
		Timezone:     "Europe/Helsinki",
		CreatedAt:    now,
		UpdatedAt:    now.Add(time.Minute),
	}
}

func assertUsersEqual(t *testing.T, got domain.User, want domain.User) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %s, want %s", got.ID, want.ID)
	}
	if got.Email != want.Email {
		t.Fatalf("Email = %q, want %q", got.Email, want.Email)
	}
	if got.Username != want.Username {
		t.Fatalf("Username = %q, want %q", got.Username, want.Username)
	}
	if got.PasswordHash != want.PasswordHash {
		t.Fatalf("PasswordHash = %q, want %q", got.PasswordHash, want.PasswordHash)
	}
	if got.Timezone != want.Timezone {
		t.Fatalf("Timezone = %q, want %q", got.Timezone, want.Timezone)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want %s", got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("UpdatedAt = %s, want %s", got.UpdatedAt, want.UpdatedAt)
	}
}
