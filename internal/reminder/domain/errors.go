package domain

import "errors"

var (
	ErrInvalidReminderID        = errors.New("invalid reminder id")
	ErrInvalidTaskID            = errors.New("invalid reminder task id")
	ErrInvalidKind              = errors.New("invalid reminder kind")
	ErrInvalidOffsetSeconds     = errors.New("invalid reminder offset seconds")
	ErrInvalidTriggerAt         = errors.New("invalid reminder trigger time")
	ErrInvalidState             = errors.New("invalid reminder state")
	ErrInvalidCreatedAt         = errors.New("invalid reminder created time")
	ErrInvalidSentAt            = errors.New("invalid reminder sent time")
	ErrInvalidReminderFields    = errors.New("invalid reminder fields")
	ErrInvalidReminderLifecycle = errors.New("invalid reminder lifecycle")
	ErrTaskDeadlineRequired     = errors.New("task deadline required")
	ErrTaskAlreadyCompleted     = errors.New("task already completed")
	ErrReminderNotFound         = errors.New("reminder not found")
)
