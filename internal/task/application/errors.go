package application

import "errors"

var (
	ErrInvalidActor         = errors.New("invalid actor")
	ErrTaskAccessDenied     = errors.New("task access denied")
	ErrAssignmentDenied     = errors.New("assignment denied")
	ErrInvalidTaskListScope = errors.New("invalid task list scope")
)
