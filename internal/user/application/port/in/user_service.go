package in

import (
	"context"

	"go_pet_project/internal/user/application/query"
)

type UserService interface {
	GetMe(ctx context.Context, query query.GetMeQuery) (query.GetMeResult, error)
}
