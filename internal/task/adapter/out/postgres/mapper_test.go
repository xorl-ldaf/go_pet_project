package postgres

import (
	"errors"
	"testing"
	"time"

	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

func TestTaskMapperRoundTripFull(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	seriesID := uuid.MustParse("10000000-0000-4000-8000-000000000011")
	deadlineAt := now.Add(24 * time.Hour)
	completedAt := now.Add(2 * time.Hour)
	archivedAt := now.Add(3 * time.Hour)
	task := restoredTask(t, domain.StatusDone, &seriesID, &deadlineAt, &completedAt, &archivedAt)

	model, err := toModel(task)
	if err != nil {
		t.Fatalf("toModel: %v", err)
	}

	if model.SeriesID == nil || *model.SeriesID != seriesID {
		t.Fatalf("SeriesID = %v, want %s", model.SeriesID, seriesID)
	}
	if model.Status != string(domain.StatusDone) {
		t.Fatalf("Status = %s, want DONE", model.Status)
	}

	got, err := toDomain(model)
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}

	assertTasksEqual(t, got, task)
}

func TestTaskMapperRoundTripNullableFields(t *testing.T) {
	task := restoredTask(t, domain.StatusOpen, nil, nil, nil, nil)

	model, err := toModel(task)
	if err != nil {
		t.Fatalf("toModel: %v", err)
	}
	if model.SeriesID != nil || model.DeadlineAt != nil || model.CompletedAt != nil || model.ArchivedAt != nil {
		t.Fatalf("nullable model fields should remain nil: %#v", model)
	}

	got, err := toDomain(model)
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}

	assertTasksEqual(t, got, task)
}

func TestTaskMapperRejectsInvalidPersistedStatus(t *testing.T) {
	model := taskModel{
		ID:          uuid.MustParse("10000000-0000-4000-8000-000000000010"),
		CreatorID:   uuid.MustParse("10000000-0000-4000-8000-000000000001"),
		AssigneeID:  uuid.MustParse("10000000-0000-4000-8000-000000000002"),
		Title:       "Invalid status",
		Description: "",
		Status:      "BROKEN",
		CreatedAt:   time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}

	_, err := toDomain(model)
	if !errors.Is(err, domain.ErrInvalidStatus) {
		t.Fatalf("toDomain error = %v, want ErrInvalidStatus", err)
	}
}

func restoredTask(
	t *testing.T,
	status domain.Status,
	seriesID *uuid.UUID,
	deadlineAt *time.Time,
	completedAt *time.Time,
	archivedAt *time.Time,
) domain.Task {
	t.Helper()

	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	task, err := domain.RestoreTask(
		uuid.MustParse("10000000-0000-4000-8000-000000000010"),
		seriesID,
		uuid.MustParse("10000000-0000-4000-8000-000000000001"),
		uuid.MustParse("10000000-0000-4000-8000-000000000002"),
		"Mapped task",
		"description",
		status,
		deadlineAt,
		now,
		now.Add(time.Hour),
		completedAt,
		archivedAt,
	)
	if err != nil {
		t.Fatalf("RestoreTask: %v", err)
	}

	return task
}

func assertTasksEqual(t *testing.T, got domain.Task, want domain.Task) {
	t.Helper()

	if got.ID != want.ID ||
		got.CreatorID != want.CreatorID ||
		got.AssigneeID != want.AssigneeID ||
		got.Title != want.Title ||
		got.Description != want.Description ||
		got.Status != want.Status ||
		!got.CreatedAt.Equal(want.CreatedAt) ||
		!got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("task mismatch:\ngot  %#v\nwant %#v", got, want)
	}
	assertUUIDPtrEqual(t, got.SeriesID, want.SeriesID)
	assertTimePtrEqual(t, got.DeadlineAt, want.DeadlineAt)
	assertTimePtrEqual(t, got.CompletedAt, want.CompletedAt)
	assertTimePtrEqual(t, got.ArchivedAt, want.ArchivedAt)
}

func assertUUIDPtrEqual(t *testing.T, got *uuid.UUID, want *uuid.UUID) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("uuid pointer mismatch: got %v want %v", got, want)
	case *got != *want:
		t.Fatalf("uuid pointer = %s, want %s", *got, *want)
	}
}

func assertTimePtrEqual(t *testing.T, got *time.Time, want *time.Time) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("time pointer mismatch: got %v want %v", got, want)
	case !got.Equal(*want):
		t.Fatalf("time pointer = %s, want %s", *got, *want)
	}
}
