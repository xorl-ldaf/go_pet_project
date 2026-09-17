package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go_pet_project/internal/notification/domain"
	"go_pet_project/internal/platform/database"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDeliveryRepositoryLifecycleAndSkipLocked(t *testing.T) {
	db := openRepositoryTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	userID, notificationID := createRepositoryFixtures(t, db, now)
	defer cleanupRepositoryFixtures(t, db, userID, notificationID)

	repo := NewDeliveryRepository(db)
	delivery, err := domain.NewNotificationDelivery(uuid.New(), notificationID, domain.DeliveryChannelTelegram, now)
	if err != nil {
		t.Fatalf("NewNotificationDelivery: %v", err)
	}
	if _, err := repo.Create(ctx, delivery); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tx1 := db.Begin()
	if tx1.Error != nil {
		t.Fatalf("begin tx1: %v", tx1.Error)
	}
	defer tx1.Rollback()
	claimed1, err := repo.ClaimDue(database.ContextWithGORM(ctx, tx1), now, 10)
	if err != nil {
		t.Fatalf("ClaimDue tx1: %v", err)
	}
	if len(claimed1) != 1 {
		t.Fatalf("tx1 claimed %d deliveries, want 1", len(claimed1))
	}

	tx2 := db.Begin()
	if tx2.Error != nil {
		t.Fatalf("begin tx2: %v", tx2.Error)
	}
	defer tx2.Rollback()
	claimed2, err := repo.ClaimDue(database.ContextWithGORM(ctx, tx2), now, 10)
	if err != nil {
		t.Fatalf("ClaimDue tx2: %v", err)
	}
	if len(claimed2) != 0 {
		t.Fatalf("tx2 claimed locked delivery count = %d, want 0", len(claimed2))
	}
	if err := tx1.Rollback().Error; err != nil {
		t.Fatalf("rollback tx1: %v", err)
	}
	if err := tx2.Rollback().Error; err != nil {
		t.Fatalf("rollback tx2: %v", err)
	}

	if err := repo.MarkSent(ctx, delivery.ID, 1, now.Add(time.Second)); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if err := repo.ScheduleRetry(ctx, delivery.ID, 2, now.Add(time.Minute), "temporary failure"); err != nil {
		t.Fatalf("ScheduleRetry: %v", err)
	}
	if err := repo.MarkFailed(ctx, delivery.ID, 3, "budget exhausted"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
}

func TestTelegramLinkTokenRepositoryConsumeValidExpiredUsed(t *testing.T) {
	db := openRepositoryTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

	userID, notificationID := createRepositoryFixtures(t, db, now)
	defer cleanupRepositoryFixtures(t, db, userID, notificationID)

	repo := NewTelegramLinkTokenRepository(db)
	token, err := domain.NewTelegramLinkToken(uuid.New(), userID, "hash-valid", now.Add(time.Minute), now)
	if err != nil {
		t.Fatalf("NewTelegramLinkToken valid: %v", err)
	}
	if _, err := repo.Create(ctx, token); err != nil {
		t.Fatalf("Create valid token: %v", err)
	}
	if _, err := repo.ConsumeByHash(ctx, "hash-valid", now); err != nil {
		t.Fatalf("ConsumeByHash valid: %v", err)
	}
	if _, err := repo.ConsumeByHash(ctx, "hash-valid", now); err == nil {
		t.Fatal("ConsumeByHash used token error = nil, want error")
	}

	expired, err := domain.NewTelegramLinkToken(uuid.New(), userID, "hash-expired", now.Add(-time.Second), now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("NewTelegramLinkToken expired: %v", err)
	}
	if _, err := repo.Create(ctx, expired); err != nil {
		t.Fatalf("Create expired token: %v", err)
	}
	if _, err := repo.ConsumeByHash(ctx, "hash-expired", now); err == nil {
		t.Fatal("ConsumeByHash expired token error = nil, want error")
	}
}

func openRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	return db
}

func createRepositoryFixtures(t *testing.T, db *gorm.DB, now time.Time) (uuid.UUID, uuid.UUID) {
	t.Helper()
	userID := uuid.New()
	notificationID := uuid.New()
	email := "telegram-stage17-" + userID.String() + "@example.com"
	username := "telegram_stage17_" + strings.ReplaceAll(userID.String(), "-", "")
	if err := db.Exec(`
		INSERT INTO users (id, email, username, password_hash, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, userID, email, username, "hash", "UTC", now, now).Error; err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			t.Skipf("database schema is not migrated: %v", err)
		}
		t.Fatalf("insert user fixture: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO notifications (id, user_id, type, title, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, notificationID, userID, string(domain.TypeTaskReminder), "Task reminder", "A task reminder is due.", now).Error; err != nil {
		t.Fatalf("insert notification fixture: %v", err)
	}

	return userID, notificationID
}

func cleanupRepositoryFixtures(t *testing.T, db *gorm.DB, userID uuid.UUID, notificationID uuid.UUID) {
	t.Helper()
	_ = db.Exec("DELETE FROM notification_deliveries WHERE notification_id = ?", notificationID).Error
	_ = db.Exec("DELETE FROM notifications WHERE id = ?", notificationID).Error
	_ = db.Exec("DELETE FROM telegram_link_tokens WHERE user_id = ?", userID).Error
	_ = db.Exec("DELETE FROM telegram_links WHERE user_id = ?", userID).Error
	_ = db.Exec("DELETE FROM users WHERE id = ?", userID).Error
}
