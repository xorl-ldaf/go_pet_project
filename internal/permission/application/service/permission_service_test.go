package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go_pet_project/internal/permission/domain"

	"github.com/google/uuid"
)

var errFakePermissionRepository = errors.New("fake permission repository error")

func TestCanAssignSelfWithoutRepository(t *testing.T) {
	repo := &fakePermissionRepository{}
	service := newTestPermissionService(t, repo)
	assignerID := testPermissionUUID(1)

	allowed, err := service.CanAssign(context.Background(), assignerID, assignerID)
	if err != nil {
		t.Fatalf("CanAssign: %v", err)
	}
	if !allowed {
		t.Fatalf("CanAssign self = false, want true")
	}
	if repo.existsCalls != 0 {
		t.Fatalf("Exists calls = %d, want 0", repo.existsCalls)
	}
}

func TestCanAssignAllowed(t *testing.T) {
	repo := &fakePermissionRepository{existsResult: true}
	service := newTestPermissionService(t, repo)
	assignerID := testPermissionUUID(1)
	assigneeID := testPermissionUUID(2)

	allowed, err := service.CanAssign(context.Background(), assignerID, assigneeID)
	if err != nil {
		t.Fatalf("CanAssign: %v", err)
	}
	if !allowed {
		t.Fatalf("CanAssign = false, want true")
	}
	if repo.lastExistsAssignerID != assignerID || repo.lastExistsAssigneeID != assigneeID {
		t.Fatalf("Exists args = %s -> %s, want %s -> %s", repo.lastExistsAssignerID, repo.lastExistsAssigneeID, assignerID, assigneeID)
	}
}

func TestCanAssignDenied(t *testing.T) {
	repo := &fakePermissionRepository{existsResult: false}
	service := newTestPermissionService(t, repo)

	allowed, err := service.CanAssign(context.Background(), testPermissionUUID(1), testPermissionUUID(3))
	if err != nil {
		t.Fatalf("CanAssign: %v", err)
	}
	if allowed {
		t.Fatalf("CanAssign = true, want false")
	}
}

func TestCanAssignRepositoryError(t *testing.T) {
	repo := &fakePermissionRepository{existsErr: errFakePermissionRepository}
	service := newTestPermissionService(t, repo)

	allowed, err := service.CanAssign(context.Background(), testPermissionUUID(1), testPermissionUUID(2))
	if !errors.Is(err, errFakePermissionRepository) {
		t.Fatalf("CanAssign error = %v, want repository error", err)
	}
	if allowed {
		t.Fatalf("CanAssign = true, want false")
	}
}

func TestCanAssignInvalidIDs(t *testing.T) {
	service := newTestPermissionService(t, &fakePermissionRepository{})

	if _, err := service.CanAssign(context.Background(), uuid.Nil, testPermissionUUID(2)); !errors.Is(err, domain.ErrInvalidAssignerID) {
		t.Fatalf("nil assigner error = %v, want ErrInvalidAssignerID", err)
	}
	if _, err := service.CanAssign(context.Background(), testPermissionUUID(1), uuid.Nil); !errors.Is(err, domain.ErrInvalidAssigneeID) {
		t.Fatalf("nil assignee error = %v, want ErrInvalidAssigneeID", err)
	}
}

func TestListAssignableUserIDsIncludesSelf(t *testing.T) {
	assignerID := testPermissionUUID(1)
	repo := &fakePermissionRepository{
		listResult: []uuid.UUID{testPermissionUUID(2), testPermissionUUID(3)},
	}
	service := newTestPermissionService(t, repo)

	ids, err := service.ListAssignableUserIDs(context.Background(), assignerID)
	if err != nil {
		t.Fatalf("ListAssignableUserIDs: %v", err)
	}

	want := []uuid.UUID{assignerID, testPermissionUUID(2), testPermissionUUID(3)}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

func TestListAssignableUserIDsDeduplicatesSelf(t *testing.T) {
	assignerID := testPermissionUUID(1)
	repo := &fakePermissionRepository{
		listResult: []uuid.UUID{assignerID, testPermissionUUID(2), assignerID},
	}
	service := newTestPermissionService(t, repo)

	ids, err := service.ListAssignableUserIDs(context.Background(), assignerID)
	if err != nil {
		t.Fatalf("ListAssignableUserIDs: %v", err)
	}

	want := []uuid.UUID{assignerID, testPermissionUUID(2)}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

func TestListAssignableUserIDsRepositoryError(t *testing.T) {
	repo := &fakePermissionRepository{listErr: errFakePermissionRepository}
	service := newTestPermissionService(t, repo)

	_, err := service.ListAssignableUserIDs(context.Background(), testPermissionUUID(1))
	if !errors.Is(err, errFakePermissionRepository) {
		t.Fatalf("ListAssignableUserIDs error = %v, want repository error", err)
	}
}

func newTestPermissionService(t *testing.T, repo *fakePermissionRepository) *PermissionService {
	t.Helper()

	service, err := NewPermissionService(repo)
	if err != nil {
		t.Fatalf("new permission service: %v", err)
	}

	return service
}

type fakePermissionRepository struct {
	existsResult bool
	existsErr    error
	existsCalls  int

	lastExistsAssignerID uuid.UUID
	lastExistsAssigneeID uuid.UUID

	listResult []uuid.UUID
	listErr    error
	listCalls  int
}

func (r *fakePermissionRepository) Exists(_ context.Context, assignerID uuid.UUID, assigneeID uuid.UUID) (bool, error) {
	r.existsCalls++
	r.lastExistsAssignerID = assignerID
	r.lastExistsAssigneeID = assigneeID
	if r.existsErr != nil {
		return false, r.existsErr
	}

	return r.existsResult, nil
}

func (r *fakePermissionRepository) ListAssigneeIDs(_ context.Context, assignerID uuid.UUID) ([]uuid.UUID, error) {
	r.listCalls++
	if r.listErr != nil {
		return nil, r.listErr
	}

	return r.listResult, nil
}

func testPermissionUUID(n int) uuid.UUID {
	return uuid.MustParse("10000000-0000-4000-8000-" + leftPadPermissionInt(n, 12))
}

func leftPadPermissionInt(n int, width int) string {
	value := ""
	for n > 0 {
		value = string(rune('0'+n%10)) + value
		n /= 10
	}
	for len(value) < width {
		value = "0" + value
	}

	return value
}
