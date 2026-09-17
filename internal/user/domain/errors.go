package domain

import "errors"

var (
	ErrUserNotFound          = errors.New("user not found")
	ErrEmailAlreadyExists    = errors.New("user email already exists")
	ErrUsernameAlreadyExists = errors.New("username already exists")
)
