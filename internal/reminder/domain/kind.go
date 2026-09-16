package domain

import "fmt"

type Kind string

const (
	KindAbsolute       Kind = "ABSOLUTE"
	KindBeforeDeadline Kind = "BEFORE_DEADLINE"
)

func ParseKind(value string) (Kind, error) {
	kind := Kind(value)
	if !kind.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidKind, value)
	}

	return kind, nil
}

func (k Kind) IsValid() bool {
	switch k {
	case KindAbsolute, KindBeforeDeadline:
		return true
	default:
		return false
	}
}
