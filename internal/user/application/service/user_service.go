package service

import (
	"context"
	"errors"
	"fmt"

	userin "go_pet_project/internal/user/application/port/in"
	userout "go_pet_project/internal/user/application/port/out"
	"go_pet_project/internal/user/application/query"
	"go_pet_project/internal/user/domain"
)

var _ userin.UserService = (*UserService)(nil)

type UserService struct {
	users userout.UserRepository
}

func NewUserService(users userout.UserRepository) (*UserService, error) {
	if users == nil {
		return nil, errors.New("user repository is required")
	}

	return &UserService{users: users}, nil
}

func (s *UserService) GetMe(ctx context.Context, q query.GetMeQuery) (query.GetMeResult, error) {
	user, err := s.users.FindByID(ctx, q.UserID)
	if err != nil {
		return query.GetMeResult{}, fmt.Errorf("get current user: %w", err)
	}

	return toGetMeResult(user), nil
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
