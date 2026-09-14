package application

import "errors"

var (
	ErrInvalidActor              = errors.New("invalid actor")
	ErrTaskAccessDenied          = errors.New("task access denied")
	ErrAssignmentNotSupportedYet = errors.New("assignment to another user is not supported yet")
	ErrInvalidTaskListScope      = errors.New("invalid task list scope")
)
