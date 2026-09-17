package command

import "time"

type RefreshCommand struct {
	RefreshToken string
}

type RefreshResult struct {
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
}
