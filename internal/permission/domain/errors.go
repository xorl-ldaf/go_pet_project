package domain

import "errors"

var (
	ErrInvalidAssignerID = errors.New("invalid assigner id")
	ErrInvalidAssigneeID = errors.New("invalid assignee id")
)
