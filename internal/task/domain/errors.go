package domain

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidTaskID           = errors.New("invalid task id")
	ErrInvalidSeriesID         = errors.New("invalid task series id")
	ErrInvalidCreatorID        = errors.New("invalid creator id")
	ErrInvalidAssigneeID       = errors.New("invalid assignee id")
	ErrInvalidTitle            = errors.New("invalid task title")
	ErrInvalidStatus           = errors.New("invalid task status")
	ErrInvalidStatusTransition = errors.New("invalid task status transition")
	ErrInvalidTimestamp        = errors.New("invalid task timestamp")
	ErrTaskNotFound            = errors.New("task not found")
)

type StatusTransitionError struct {
	From Status
	To   Status
}

func (e StatusTransitionError) Error() string {
	return fmt.Sprintf("%s: %s -> %s", ErrInvalidStatusTransition, e.From, e.To)
}

func (e StatusTransitionError) Unwrap() error {
	return ErrInvalidStatusTransition
}
