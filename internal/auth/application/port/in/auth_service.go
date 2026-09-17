package in

import (
	"context"

	"go_pet_project/internal/auth/application/command"
)

type AuthService interface {
	Register(ctx context.Context, cmd command.RegisterCommand) (command.RegisterResult, error)
	Login(ctx context.Context, cmd command.LoginCommand) (command.LoginResult, error)
	Refresh(ctx context.Context, cmd command.RefreshCommand) (command.RefreshResult, error)
	Logout(ctx context.Context, cmd command.LogoutCommand) (command.LogoutResult, error)
}
