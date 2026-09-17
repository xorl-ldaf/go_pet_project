package domain

import "fmt"

type State string

const (
	StatePending   State = "PENDING"
	StateSent      State = "SENT"
	StateCancelled State = "CANCELLED"
)

const InitialState = StatePending

func ParseState(value string) (State, error) {
	state := State(value)
	if !state.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidState, value)
	}

	return state, nil
}

func (s State) IsValid() bool {
	switch s {
	case StatePending, StateSent, StateCancelled:
		return true
	default:
		return false
	}
}

func (s State) IsPending() bool {
	return s == StatePending
}
