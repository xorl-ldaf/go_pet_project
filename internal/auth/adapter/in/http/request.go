package httpadapter

import "go_pet_project/internal/auth/application/command"

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	Timezone string `json:"timezone"`
}

func (r RegisterRequest) Command() command.RegisterCommand {
	return command.RegisterCommand{
		Email:    r.Email,
		Username: r.Username,
		Password: r.Password,
		Timezone: r.Timezone,
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r LoginRequest) Command() command.LoginCommand {
	return command.LoginCommand{
		Email:    r.Email,
		Password: r.Password,
	}
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (r RefreshRequest) Command() command.RefreshCommand {
	return command.RefreshCommand{
		RefreshToken: r.RefreshToken,
	}
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (r LogoutRequest) Command() command.LogoutCommand {
	return command.LogoutCommand{
		RefreshToken: r.RefreshToken,
	}
}
