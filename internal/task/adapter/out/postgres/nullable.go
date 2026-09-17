package postgres

import (
	"time"

	"github.com/google/uuid"
)

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copied := *value
	return &copied
}

func cloneUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}

	copied := *value
	return &copied
}
