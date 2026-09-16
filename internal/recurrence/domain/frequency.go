package domain

import "fmt"

type Frequency string

const (
	FrequencyDaily   Frequency = "DAILY"
	FrequencyWeekly  Frequency = "WEEKLY"
	FrequencyMonthly Frequency = "MONTHLY"
)

func ParseFrequency(value string) (Frequency, error) {
	frequency := Frequency(value)
	if !frequency.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidFrequency, value)
	}

	return frequency, nil
}

func (f Frequency) IsValid() bool {
	switch f {
	case FrequencyDaily, FrequencyWeekly, FrequencyMonthly:
		return true
	default:
		return false
	}
}
