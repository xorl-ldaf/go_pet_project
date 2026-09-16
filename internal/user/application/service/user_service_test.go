package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go_pet_project/internal/user/application"
	"go_pet_project/internal/user/application/query"
	"go_pet_project/internal/user/domain"

	"github.com/google/uuid"
)

var errFakeUserRepository = errors.New("fake user repository error")
var errFakeAssignableUserProvider = errors.New("fake assignable user provider error")

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

func TestListAssignableUsersSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 23, 30, 0, 0, time.UTC)
	actorID := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	otherID := uuid.MustParse("10000000-0000-4000-8000-000000000002")
	users := &fakeUserRepository{
		findByIDsUsers: []domain.User{
			{
				ID:           actorID,
				Email:        "a@example.com",
				Username:     "user_a",
				PasswordHash: "hash-a",
				Timezone:     "UTC",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           otherID,
				Email:        "b@example.com",
				Username:     "user_b",
				PasswordHash: "hash-b",
				Timezone:     "UTC",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}
	assignableUsers := &fakeAssignableUserProvider{ids: []uuid.UUID{actorID, otherID}}
	service := newTestUserServiceWithProvider(t, users, assignableUsers)

	result, err := service.ListAssignableUsers(ctx, query.ListAssignableUsersQuery{ActorID: actorID})
	if err != nil {
		t.Fatalf("ListAssignableUsers: %v", err)
	}

	if assignableUsers.calls != 1 || assignableUsers.lastAssignerID != actorID {
		t.Fatalf("provider calls=%d assigner=%s", assignableUsers.calls, assignableUsers.lastAssignerID)
	}
	if users.findByIDsCalls != 1 || len(users.lastFindByIDs) != 2 || users.lastFindByIDs[0] != actorID || users.lastFindByIDs[1] != otherID {
		t.Fatalf("FindByIDs ids = %v, want [%s %s]", users.lastFindByIDs, actorID, otherID)
	}
	if len(result.Users) != 2 ||
		result.Users[0].ID != actorID ||
		result.Users[0].Email != "a@example.com" ||
		result.Users[1].ID != otherID ||
		result.Users[1].Username != "user_b" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestListAssignableUsersInvalidActor(t *testing.T) {
	service := newTestUserService(t, &fakeUserRepository{})

	_, err := service.ListAssignableUsers(context.Background(), query.ListAssignableUsersQuery{})
	if !errors.Is(err, application.ErrInvalidActor) {
		t.Fatalf("ListAssignableUsers error = %v, want ErrInvalidActor", err)
	}
}

func TestListAssignableUsersProviderFailure(t *testing.T) {
	users := &fakeUserRepository{}
	service := newTestUserServiceWithProvider(t, users, &fakeAssignableUserProvider{err: errFakeAssignableUserProvider})

	_, err := service.ListAssignableUsers(context.Background(), query.ListAssignableUsersQuery{ActorID: uuid.New()})
	if !errors.Is(err, errFakeAssignableUserProvider) {
		t.Fatalf("ListAssignableUsers error = %v, want provider error", err)
	}
	if users.findByIDsCalls != 0 {
		t.Fatalf("FindByIDs calls = %d, want 0", users.findByIDsCalls)
	}
}

func TestListAssignableUsersRepositoryFailure(t *testing.T) {
	users := &fakeUserRepository{findByIDsErr: errFakeUserRepository}
	service := newTestUserServiceWithProvider(t, users, &fakeAssignableUserProvider{ids: []uuid.UUID{uuid.New()}})

	_, err := service.ListAssignableUsers(context.Background(), query.ListAssignableUsersQuery{ActorID: uuid.New()})
	if !errors.Is(err, errFakeUserRepository) {
		t.Fatalf("ListAssignableUsers error = %v, want repository error", err)
	}
}

func newTestUserService(t *testing.T, users *fakeUserRepository) *UserService {
	t.Helper()

	return newTestUserServiceWithProvider(t, users, &fakeAssignableUserProvider{})
}

func newTestUserServiceWithProvider(t *testing.T, users *fakeUserRepository, assignableUsers *fakeAssignableUserProvider) *UserService {
	t.Helper()

	service, err := NewUserService(users, assignableUsers)
	if err != nil {
		t.Fatalf("new user service: %v", err)
	}

	return service
}

type fakeUserRepository struct {
	findByIDUser   domain.User
	findByIDErr    error
	findByIDCalls  int
	findByIDsUsers []domain.User
	findByIDsErr   error
	findByIDsCalls int
	lastFindByID   uuid.UUID
	lastFindByIDs  []uuid.UUID
	lastCtx        context.Context
}

func (r *fakeUserRepository) Create(context.Context, domain.User) (domain.User, error) {
	return domain.User{}, errFakeUserRepository
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

func (r *fakeUserRepository) FindByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.User, error) {
	r.findByIDsCalls++
	r.lastCtx = ctx
	r.lastFindByIDs = append([]uuid.UUID(nil), ids...)
	if r.findByIDsErr != nil {
		return nil, r.findByIDsErr
	}

	return r.findByIDsUsers, nil
}

func (r *fakeUserRepository) FindByEmail(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrUserNotFound
}

func (r *fakeUserRepository) FindByUsername(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrUserNotFound
}

type fakeAssignableUserProvider struct {
	ids []uuid.UUID
	err error

	calls          int
	lastAssignerID uuid.UUID
}

func (p *fakeAssignableUserProvider) ListAssignableUserIDs(_ context.Context, assignerID uuid.UUID) ([]uuid.UUID, error) {
	p.calls++
	p.lastAssignerID = assignerID
	if p.err != nil {
		return nil, p.err
	}

	return p.ids, nil
}
