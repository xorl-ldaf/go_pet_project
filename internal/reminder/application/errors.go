package application

import "errors"

var (
	ErrInvalidActor         = errors.New("invalid actor")
	ErrReminderAccessDenied = errors.New("reminder access denied")
)
