package domain

import "errors"

var (
	ErrInvalidNotificationID = errors.New("invalid notification id")
	ErrInvalidUserID         = errors.New("invalid notification user id")
	ErrInvalidTaskID         = errors.New("invalid notification task id")
	ErrInvalidReminderID     = errors.New("invalid notification reminder id")
	ErrInvalidType           = errors.New("invalid notification type")
	ErrInvalidTitle          = errors.New("invalid notification title")
	ErrInvalidBody           = errors.New("invalid notification body")
	ErrInvalidCreatedAt      = errors.New("invalid notification created time")
	ErrInvalidReadAt         = errors.New("invalid notification read time")
	ErrNotificationNotFound  = errors.New("notification not found")

	ErrInvalidDeliveryID          = errors.New("invalid notification delivery id")
	ErrInvalidDeliveryChannel     = errors.New("invalid notification delivery channel")
	ErrInvalidDeliveryStatus      = errors.New("invalid notification delivery status")
	ErrInvalidDeliveryAttempts    = errors.New("invalid notification delivery attempts")
	ErrInvalidDeliveryNextAttempt = errors.New("invalid notification delivery next attempt")
	ErrInvalidDeliverySentAt      = errors.New("invalid notification delivery sent time")
	ErrDeliveryNotFound           = errors.New("notification delivery not found")

	ErrInvalidTelegramChatID             = errors.New("invalid telegram chat id")
	ErrInvalidTelegramLinkedAt           = errors.New("invalid telegram linked time")
	ErrInvalidTelegramLinkTokenID        = errors.New("invalid telegram link token id")
	ErrInvalidTelegramLinkToken          = errors.New("invalid telegram link token")
	ErrInvalidTelegramLinkTokenExpiry    = errors.New("invalid telegram link token expiry")
	ErrInvalidTelegramLinkTokenUsedAt    = errors.New("invalid telegram link token used time")
	ErrTelegramLinkNotFound              = errors.New("telegram link not found")
	ErrTelegramLinkTokenNotFound         = errors.New("telegram link token not found")
	ErrTelegramLinkTokenInvalidOrExpired = errors.New("telegram link token invalid or expired")
)
