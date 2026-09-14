package domain

import (
	"errors"
	"testing"
)

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  Status
	}{
		{name: "open", value: "OPEN", want: StatusOpen},
		{name: "in progress", value: "IN_PROGRESS", want: StatusInProgress},
		{name: "done", value: "DONE", want: StatusDone},
		{name: "cancelled", value: "CANCELLED", want: StatusCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatus(tt.value)
			if err != nil {
				t.Fatalf("ParseStatus: %v", err)
			}
			if got != tt.want {
				t.Fatalf("status = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseStatusRejectsInvalidStatus(t *testing.T) {
	_, err := ParseStatus("ANYTHING")
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("ParseStatus error = %v, want ErrInvalidStatus", err)
	}
}

func TestStatusTransitionsMatchJavaReference(t *testing.T) {
	statuses := []Status{
		StatusOpen,
		StatusInProgress,
		StatusDone,
		StatusCancelled,
	}
	allowed := map[Status]map[Status]bool{
		StatusOpen: {
			StatusInProgress: true,
			StatusDone:       true,
		},
		StatusInProgress: {
			StatusDone: true,
		},
	}

	for _, from := range statuses {
		for _, to := range statuses {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				want := allowed[from][to]
				if got := from.CanTransitionTo(to); got != want {
					t.Fatalf("CanTransitionTo(%s, %s) = %v, want %v", from, to, got, want)
				}
			})
		}
	}
}

func TestInvalidStatusCannotTransition(t *testing.T) {
	if Status("BROKEN").CanTransitionTo(StatusOpen) {
		t.Fatalf("invalid source status must not transition")
	}
	if StatusOpen.CanTransitionTo(Status("BROKEN")) {
		t.Fatalf("invalid target status must not transition")
	}
}
