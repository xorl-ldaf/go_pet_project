package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TaskSeries struct {
	ID             uuid.UUID
	CreatorID      uuid.UUID
	AssigneeID     uuid.UUID
	Title          string
	Description    string
	Frequency      Frequency
	Interval       int
	NextDeadlineAt time.Time
	Timezone       string
	EndsAt         *time.Time
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewTaskSeries(
	creatorID uuid.UUID,
	assigneeID uuid.UUID,
	title string,
	description string,
	frequency Frequency,
	interval int,
	nextDeadlineAt time.Time,
	timezone string,
	endsAt *time.Time,
	now time.Time,
) (TaskSeries, error) {
	if assigneeID == uuid.Nil {
		assigneeID = creatorID
	}

	return RestoreTaskSeries(
		uuid.New(),
		creatorID,
		assigneeID,
		title,
		description,
		frequency,
		interval,
		nextDeadlineAt,
		timezone,
		endsAt,
		true,
		now,
		now,
	)
}

func RestoreTaskSeries(
	id uuid.UUID,
	creatorID uuid.UUID,
	assigneeID uuid.UUID,
	title string,
	description string,
	frequency Frequency,
	interval int,
	nextDeadlineAt time.Time,
	timezone string,
	endsAt *time.Time,
	isActive bool,
	createdAt time.Time,
	updatedAt time.Time,
) (TaskSeries, error) {
	if id == uuid.Nil {
		return TaskSeries{}, ErrInvalidSeriesID
	}
	if creatorID == uuid.Nil {
		return TaskSeries{}, ErrInvalidCreatorID
	}
	if assigneeID == uuid.Nil {
		return TaskSeries{}, ErrInvalidAssigneeID
	}
	if strings.TrimSpace(title) == "" {
		return TaskSeries{}, ErrInvalidTitle
	}
	if !frequency.IsValid() {
		return TaskSeries{}, fmt.Errorf("%w: %s", ErrInvalidFrequency, frequency)
	}
	if interval <= 0 {
		return TaskSeries{}, ErrInvalidInterval
	}
	if nextDeadlineAt.IsZero() {
		return TaskSeries{}, ErrInvalidNextDeadlineAt
	}
	if _, err := loadLocation(timezone); err != nil {
		return TaskSeries{}, err
	}
	if createdAt.IsZero() || updatedAt.IsZero() || updatedAt.Before(createdAt) {
		return TaskSeries{}, ErrInvalidTimestamp
	}
	if endsAt != nil && endsAt.IsZero() {
		return TaskSeries{}, ErrInvalidTimestamp
	}

	return TaskSeries{
		ID:             id,
		CreatorID:      creatorID,
		AssigneeID:     assigneeID,
		Title:          title,
		Description:    description,
		Frequency:      frequency,
		Interval:       interval,
		NextDeadlineAt: nextDeadlineAt,
		Timezone:       timezone,
		EndsAt:         cloneTimePtr(endsAt),
		IsActive:       isActive,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, nil
}

func (s TaskSeries) NextDeadline(current time.Time) (time.Time, error) {
	if current.IsZero() {
		return time.Time{}, ErrInvalidNextDeadlineAt
	}
	if s.Interval <= 0 {
		return time.Time{}, ErrInvalidInterval
	}

	location, err := loadLocation(s.Timezone)
	if err != nil {
		return time.Time{}, err
	}

	local := current.In(location)
	switch s.Frequency {
	case FrequencyDaily:
		return local.AddDate(0, 0, s.Interval).UTC(), nil
	case FrequencyWeekly:
		return local.AddDate(0, 0, s.Interval*7).UTC(), nil
	case FrequencyMonthly:
		return addCalendarMonths(local, s.Interval).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("%w: %s", ErrInvalidFrequency, s.Frequency)
	}
}

func (s *TaskSeries) AdvanceAfterOccurrence(now time.Time) error {
	next, err := s.NextDeadline(s.NextDeadlineAt)
	if err != nil {
		return err
	}

	if s.EndsAt != nil && next.After(*s.EndsAt) {
		s.IsActive = false
		s.UpdatedAt = now
		return nil
	}

	s.NextDeadlineAt = next
	s.UpdatedAt = now
	return nil
}

func loadLocation(timezone string) (*time.Location, error) {
	if strings.TrimSpace(timezone) == "" {
		return nil, ErrInvalidTimezone
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTimezone, timezone)
	}

	return location, nil
}

func addCalendarMonths(value time.Time, months int) time.Time {
	year, month, day := value.Date()
	hour, minute, second := value.Clock()
	nanosecond := value.Nanosecond()
	location := value.Location()

	firstOfTarget := time.Date(year, month+time.Month(months), 1, hour, minute, second, nanosecond, location)
	lastDay := time.Date(firstOfTarget.Year(), firstOfTarget.Month()+1, 0, hour, minute, second, nanosecond, location).Day()
	if day > lastDay {
		day = lastDay
	}

	return time.Date(firstOfTarget.Year(), firstOfTarget.Month(), day, hour, minute, second, nanosecond, location)
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copied := *value
	return &copied
}
