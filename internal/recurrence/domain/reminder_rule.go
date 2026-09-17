package domain

import "github.com/google/uuid"

type ReminderRule struct {
	ID            uuid.UUID
	SeriesID      uuid.UUID
	OffsetSeconds int64
}

func NewReminderRule(seriesID uuid.UUID, offsetSeconds int64) (ReminderRule, error) {
	return RestoreReminderRule(uuid.New(), seriesID, offsetSeconds)
}

func RestoreReminderRule(id uuid.UUID, seriesID uuid.UUID, offsetSeconds int64) (ReminderRule, error) {
	if id == uuid.Nil {
		return ReminderRule{}, ErrInvalidReminderRuleID
	}
	if seriesID == uuid.Nil {
		return ReminderRule{}, ErrInvalidSeriesID
	}
	if offsetSeconds <= 0 {
		return ReminderRule{}, ErrInvalidOffsetSeconds
	}

	return ReminderRule{
		ID:            id,
		SeriesID:      seriesID,
		OffsetSeconds: offsetSeconds,
	}, nil
}
