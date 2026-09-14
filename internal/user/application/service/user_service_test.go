package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go_pet_project/internal/user/application/query"
	"go_pet_project/internal/user/domain"

	"github.com/google/uuid"
)

var errFakeUserRepository = errors.New("fake user repository error")

func TestGetMeSuccess(t *testing.T) {
	ctx := context.Background()
	userID := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	now := time.Date(2026, 9, 14, 23, 0, 0, 0, time.UTC)
	users := &fakeUserRepository{
		findByIDUser: domain.User{
			ID:           userID,
			Email:        "me@example.com",
			Username:     "me",
			PasswordHash: "hashed-password",
			Timezone:     "UTC",
			CreatedAt:    now,
			UpdatedAt:    now.Add(time.Minute),
		},
	}
	service := newTestUserService(t, users)

	result, err := service.GetMe(ctx, query.GetMeQuery{UserID: userID})
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}

	if users.findByIDCalls != 1 || users.lastFindByID != userID {
		t.Fatalf("FindByID calls=%d id=%s", users.findByIDCalls, users.lastFindByID)
	}
	if users.lastCtx != ctx {
		t.Fatalf("FindByID did not receive original context")
	}
	if result.ID != userID ||
		result.Email != "me@example.com" ||
		result.Username != "me" ||
		result.Timezone != "UTC" ||
		!result.CreatedAt.Equal(now) ||
		!result.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGetMeNotFound(t *testing.T) {
	users := &fakeUserRepository{findByIDErr: domain.ErrUserNotFound}
	service := newTestUserService(t, users)

	_, err := service.GetMe(context.Background(), query.GetMeQuery{UserID: uuid.New()})
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("GetMe error = %v, want ErrUserNotFound", err)
	}
}

func TestGetMeRepositoryFailure(t *testing.T) {
	users := &fakeUserRepository{findByIDErr: errFakeUserRepository}
	service := newTestUserService(t, users)

	_, err := service.GetMe(context.Background(), query.GetMeQuery{UserID: uuid.New()})
	if !errors.Is(err, errFakeUserRepository) {
		t.Fatalf("GetMe error = %v, want repository error", err)
	}
}

func newTestUserService(t *testing.T, users *fakeUserRepository) *UserService {
	t.Helper()

	service, err := NewUserService(users)
	if err != nil {
		t.Fatalf("new user service: %v", err)
	}

	return service
}

type fakeUserRepository struct {
	findByIDUser  domain.User
	findByIDErr   error
	findByIDCalls int
	lastFindByID  uuid.UUID
	lastCtx       context.Context
}

func (r *fakeUserRepository) Create(context.Context, domain.User) (domain.User, error) {
	panic("not implemented")
}

func (r *fakeUserRepository) FindByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	r.findByIDCalls++
	r.lastFindByID = id
	r.lastCtx = ctx
	if r.findByIDErr != nil {
		return domain.User{}, r.findByIDErr
	}

	return r.findByIDUser, nil
}

func (r *fakeUserRepository) FindByEmail(context.Context, string) (domain.User, error) {
	panic("not implemented")
}

func (r *fakeUserRepository) FindByUsername(context.Context, string) (domain.User, error) {
	panic("not implemented")
}
