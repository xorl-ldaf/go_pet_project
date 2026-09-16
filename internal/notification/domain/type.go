package domain

import "fmt"

type Type string

const TypeTaskReminder Type = "TASK_REMINDER"

func ParseType(value string) (Type, error) {
	notificationType := Type(value)
	if !notificationType.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidType, value)
	}

	return notificationType, nil
}

func (t Type) IsValid() bool {
	return t == TypeTaskReminder
}
