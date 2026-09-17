package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNextDeadlineDailyInterval(t *testing.T) {
	series := mustTestSeries(t, FrequencyDaily, 2, "UTC", nil)
	current := time.Date(2026, 9, 16, 9, 30, 0, 0, time.UTC)

	next, err := series.NextDeadline(current)
	if err != nil {
		t.Fatalf("NextDeadline: %v", err)
	}

	want := time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestNextDeadlineWeeklyInterval(t *testing.T) {
	series := mustTestSeries(t, FrequencyWeekly, 2, "UTC", nil)
	current := time.Date(2026, 9, 16, 9, 30, 0, 0, time.UTC)

	next, err := series.NextDeadline(current)
	if err != nil {
		t.Fatalf("NextDeadline: %v", err)
	}

	want := time.Date(2026, 9, 30, 9, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestNextDeadlineMonthlyInterval(t *testing.T) {
	location := mustLocation(t, "Europe/Moscow")
	series := mustTestSeries(t, FrequencyMonthly, 1, "Europe/Moscow", nil)
	current := time.Date(2026, 1, 31, 10, 0, 0, 0, location)

	next, err := series.NextDeadline(current)
	if err != nil {
		t.Fatalf("NextDeadline: %v", err)
	}

	want := time.Date(2026, 2, 28, 10, 0, 0, 0, location).UTC()
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestTaskSeriesInvalidInterval(t *testing.T) {
	_, err := RestoreTaskSeries(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		"bad interval",
		"",
		FrequencyDaily,
		0,
		time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC),
		"UTC",
		nil,
		true,
		time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
	)
	if !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("err = %v, want ErrInvalidInterval", err)
	}
}

func TestNextDeadlinePreservesWallClockAcrossDST(t *testing.T) {
	location := mustLocation(t, "America/New_York")
	series := mustTestSeries(t, FrequencyDaily, 1, "America/New_York", nil)
	current := time.Date(2026, 3, 7, 9, 30, 0, 0, location)

	next, err := series.NextDeadline(current)
	if err != nil {
		t.Fatalf("NextDeadline: %v", err)
	}

	localNext := next.In(location)
	if localNext.Hour() != 9 || localNext.Minute() != 30 || localNext.Day() != 8 {
		t.Fatalf("local next = %s, want 2026-03-08 09:30 local", localNext)
	}
	if next.Sub(current.UTC()) != 23*time.Hour {
		t.Fatalf("absolute duration = %s, want 23h across spring DST", next.Sub(current.UTC()))
	}
}

func TestAdvanceAfterOccurrenceDeactivatesAfterEndsAt(t *testing.T) {
	endsAt := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	series := mustTestSeries(t, FrequencyDaily, 1, "UTC", &endsAt)
	series.NextDeadlineAt = time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 16, 9, 1, 0, 0, time.UTC)

	if err := series.AdvanceAfterOccurrence(now); err != nil {
		t.Fatalf("AdvanceAfterOccurrence: %v", err)
	}
	if series.IsActive {
		t.Fatal("series is active, want inactive")
	}
	if !series.NextDeadlineAt.Equal(time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("next deadline advanced after ends_at: %s", series.NextDeadlineAt)
	}
}

func mustTestSeries(t *testing.T, frequency Frequency, interval int, timezone string, endsAt *time.Time) TaskSeries {
	t.Helper()

	now := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	series, err := RestoreTaskSeries(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		"series",
		"description",
		frequency,
		interval,
		now,
		timezone,
		endsAt,
		true,
		now.Add(-time.Hour),
		now.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("RestoreTaskSeries: %v", err)
	}

	return series
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()

	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%s): %v", name, err)
	}

	return location
}
