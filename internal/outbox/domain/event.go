package domain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	ReminderAggregateType       = "reminder"
	NotificationRequestedV1Type = "notification.requested.v1"
	EventVersion                = 1
)

var (
	ErrInvalidEventID       = errors.New("invalid outbox event id")
	ErrInvalidAggregateID   = errors.New("invalid outbox aggregate id")
	ErrInvalidAggregateType = errors.New("invalid outbox aggregate type")
	ErrInvalidEventType     = errors.New("invalid outbox event type")
	ErrInvalidPayload       = errors.New("invalid outbox payload")
	ErrInvalidCreatedAt     = errors.New("invalid outbox created time")
	ErrInvalidPublishedAt   = errors.New("invalid outbox published time")
	ErrInvalidAttempts      = errors.New("invalid outbox attempts")
	ErrEventNotFound        = errors.New("outbox event not found")
)

type Event struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       json.RawMessage
	CreatedAt     time.Time
	PublishedAt   *time.Time
	Attempts      int
}

func NewEvent(id uuid.UUID, aggregateType string, aggregateID uuid.UUID, eventType string, payload json.RawMessage, createdAt time.Time) (Event, error) {
	return RestoreEvent(id, aggregateType, aggregateID, eventType, payload, createdAt, nil, 0)
}

func RestoreEvent(
	id uuid.UUID,
	aggregateType string,
	aggregateID uuid.UUID,
	eventType string,
	payload json.RawMessage,
	createdAt time.Time,
	publishedAt *time.Time,
	attempts int,
) (Event, error) {
	if id == uuid.Nil {
		return Event{}, ErrInvalidEventID
	}
	if aggregateType == "" {
		return Event{}, ErrInvalidAggregateType
	}
	if aggregateID == uuid.Nil {
		return Event{}, ErrInvalidAggregateID
	}
	if eventType == "" {
		return Event{}, ErrInvalidEventType
	}
	if !json.Valid(payload) {
		return Event{}, ErrInvalidPayload
	}
	if createdAt.IsZero() {
		return Event{}, ErrInvalidCreatedAt
	}
	if publishedAt != nil && publishedAt.Before(createdAt) {
		return Event{}, ErrInvalidPublishedAt
	}
	if attempts < 0 {
		return Event{}, ErrInvalidAttempts
	}

	return Event{
		ID:            id,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       cloneJSON(payload),
		CreatedAt:     createdAt,
		PublishedAt:   cloneTimePtr(publishedAt),
		Attempts:      attempts,
	}, nil
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	copied := make([]byte, len(value))
	copy(copied, value)

	return copied
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value

	return &copied
}
