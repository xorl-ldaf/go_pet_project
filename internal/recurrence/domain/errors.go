package domain

import "errors"

var (
	ErrInvalidSeriesID       = errors.New("invalid series id")
	ErrInvalidCreatorID      = errors.New("invalid creator id")
	ErrInvalidAssigneeID     = errors.New("invalid assignee id")
	ErrInvalidTitle          = errors.New("invalid title")
	ErrInvalidFrequency      = errors.New("invalid frequency")
	ErrInvalidInterval       = errors.New("invalid interval")
	ErrInvalidNextDeadlineAt = errors.New("invalid next deadline at")
	ErrInvalidTimezone       = errors.New("invalid timezone")
	ErrInvalidTimestamp      = errors.New("invalid timestamp")
	ErrInvalidReminderRuleID = errors.New("invalid reminder rule id")
	ErrInvalidOffsetSeconds  = errors.New("invalid offset seconds")
	ErrSeriesNotFound        = errors.New("series not found")
)
