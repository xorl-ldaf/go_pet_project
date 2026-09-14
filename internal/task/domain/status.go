package domain

import "fmt"

type Status string

const (
	StatusOpen       Status = "OPEN"
	StatusInProgress Status = "IN_PROGRESS"
	StatusDone       Status = "DONE"
	StatusCancelled  Status = "CANCELLED"
)

const InitialStatus = StatusOpen

func ParseStatus(value string) (Status, error) {
	status := Status(value)
	if !status.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidStatus, value)
	}

	return status, nil
}

func (s Status) IsValid() bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusDone, StatusCancelled:
		return true
	default:
		return false
	}
}

func (s Status) IsCompleted() bool {
	return s == StatusDone
}

func (s Status) CanTransitionTo(next Status) bool {
	if !s.IsValid() || !next.IsValid() {
		return false
	}

	for _, allowed := range allowedStatusTransitions[s] {
		if allowed == next {
			return true
		}
	}

	return false
}
