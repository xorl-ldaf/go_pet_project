package postgres

import (
	"testing"
	"time"

	"go_pet_project/internal/user/domain"

	"github.com/google/uuid"
)

func TestUserMapperRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 14, 16, 40, 0, 123456000, time.UTC)
	user := domain.User{
		ID:           uuid.New(),
		Email:        "mapper@example.com",
		Username:     "mapper",
		PasswordHash: "$fake-hash-for-test",
		Timezone:     "Europe/Helsinki",
		CreatedAt:    now,
		UpdatedAt:    now.Add(time.Minute),
	}

	got := toDomain(toModel(user))
	if got != user {
		t.Fatalf("round-trip user = %#v, want %#v", got, user)
	}
}
