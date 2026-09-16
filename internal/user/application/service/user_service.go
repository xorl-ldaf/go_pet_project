package service

import (
	"context"
	"errors"
	"fmt"

	"go_pet_project/internal/user/application"
	userin "go_pet_project/internal/user/application/port/in"
	userout "go_pet_project/internal/user/application/port/out"
	"go_pet_project/internal/user/application/query"
	"go_pet_project/internal/user/domain"

	"github.com/google/uuid"
)

var _ userin.UserService = (*UserService)(nil)

type UserService struct {
	users           userout.UserRepository
	assignableUsers userout.AssignableUserProvider
}

func NewUserService(users userout.UserRepository, assignableUsers userout.AssignableUserProvider) (*UserService, error) {
	if users == nil {
		return nil, errors.New("user repository is required")
	}
	if assignableUsers == nil {
		return nil, errors.New("assignable user provider is required")
	}

	return &UserService{users: users, assignableUsers: assignableUsers}, nil
}

func (s *UserService) GetMe(ctx context.Context, q query.GetMeQuery) (query.GetMeResult, error) {
	user, err := s.users.FindByID(ctx, q.UserID)
	if err != nil {
		return query.GetMeResult{}, fmt.Errorf("get current user: %w", err)
	}

	return toGetMeResult(user), nil
}

func (s *UserService) ListAssignableUsers(ctx context.Context, q query.ListAssignableUsersQuery) (query.ListAssignableUsersResult, error) {
	if q.ActorID == uuid.Nil {
		return query.ListAssignableUsersResult{}, application.ErrInvalidActor
	}

	ids, err := s.assignableUsers.ListAssignableUserIDs(ctx, q.ActorID)
	if err != nil {
		return query.ListAssignableUsersResult{}, fmt.Errorf("list assignable user ids: %w", err)
	}

	users, err := s.users.FindByIDs(ctx, ids)
	if err != nil {
		return query.ListAssignableUsersResult{}, fmt.Errorf("list assignable users: %w", err)
	}

	results := make([]query.GetMeResult, 0, len(users))
	for _, user := range users {
		results = append(results, toGetMeResult(user))
	}

	return query.ListAssignableUsersResult{Users: results}, nil
}

func toGetMeResult(user domain.User) query.GetMeResult {
	return query.GetMeResult{
		ID:        user.ID,
		Email:     user.Email,
		Username:  user.Username,
		Timezone:  user.Timezone,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
