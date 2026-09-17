package out

import (
	"context"

	userdomain "go_pet_project/internal/user/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user userdomain.User) (userdomain.User, error)
	FindByEmail(ctx context.Context, email string) (userdomain.User, error)
	FindByUsername(ctx context.Context, username string) (userdomain.User, error)
}
