package application

import "errors"

var (
	ErrInvalidActor             = errors.New("invalid actor")
	ErrNotificationAccessDenied = errors.New("notification access denied")
	ErrInvalidProcessedEvent    = errors.New("invalid processed event")
	ErrInvalidNotificationEvent = errors.New("invalid notification event")
	ErrInvalidTelegramToken     = errors.New("invalid telegram token")
)
