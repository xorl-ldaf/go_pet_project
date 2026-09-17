package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	outboxout "go_pet_project/internal/outbox/application/port/out"
	"go_pet_project/internal/outbox/domain"

	"github.com/google/uuid"
)

func TestRelayPublishSuccessMarksPublished(t *testing.T) {
	event := mustRelayTestEvent(t)
	repository := newFakeOutboxRepository(event)
	publisher := &fakePublisher{}
	relay := newTestRelay(t, repository, publisher)

	processed, err := relay.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	found := repository.events[event.ID]
	if found.PublishedAt == nil {
		t.Fatalf("PublishedAt = nil, want set")
	}
	if found.Attempts != 0 {
		t.Fatalf("Attempts = %d, want 0", found.Attempts)
	}
	if publisher.calls != 1 {
		t.Fatalf("publisher calls = %d, want 1", publisher.calls)
	}
}

func TestRelayPublishErrorIncrementsAttempts(t *testing.T) {
	event := mustRelayTestEvent(t)
	repository := newFakeOutboxRepository(event)
	publisher := &fakePublisher{err: errFakePublish}
	relay := newTestRelay(t, repository, publisher)

	processed, err := relay.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	found := repository.events[event.ID]
	if found.PublishedAt != nil {
		t.Fatalf("PublishedAt = %v, want nil", found.PublishedAt)
	}
	if found.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", found.Attempts)
	}
}

func TestRelayAtLeastOnceWhenMarkPublishedFails(t *testing.T) {
	event := mustRelayTestEvent(t)
	repository := newFakeOutboxRepository(event)
	repository.markPublishedErr = errFakeMarkPublished
	publisher := &fakePublisher{}
	relay := newTestRelay(t, repository, publisher)

	_, err := relay.ProcessOnce(context.Background())
	if !errors.Is(err, errFakeMarkPublished) {
		t.Fatalf("ProcessOnce error = %v, want mark failure", err)
	}
	if repository.events[event.ID].PublishedAt != nil {
		t.Fatalf("PublishedAt = %v, want nil after failed mark", repository.events[event.ID].PublishedAt)
	}

	repository.markPublishedErr = nil
	_, err = relay.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("retry ProcessOnce: %v", err)
	}
	if publisher.calls != 2 {
		t.Fatalf("publisher calls = %d, want 2 duplicate deliveries", publisher.calls)
	}
	if repository.events[event.ID].PublishedAt == nil {
		t.Fatalf("PublishedAt = nil after retry")
	}
}

var (
	errFakePublish       = errors.New("fake publish error")
	errFakeMarkPublished = errors.New("fake mark published error")
)

type fakePublisher struct {
	err   error
	calls int
}

func (p *fakePublisher) Publish(_ context.Context, _ domain.Event) error {
	p.calls++
	return p.err
}

type fakeOutboxRepository struct {
	events           map[uuid.UUID]domain.Event
	markPublishedErr error
}

func newFakeOutboxRepository(events ...domain.Event) *fakeOutboxRepository {
	repository := &fakeOutboxRepository{events: map[uuid.UUID]domain.Event{}}
	for _, event := range events {
		repository.events[event.ID] = event
	}

	return repository
}

func (r *fakeOutboxRepository) Create(_ context.Context, event domain.Event) (domain.Event, error) {
	r.events[event.ID] = event
	return event, nil
}

func (r *fakeOutboxRepository) ClaimUnpublished(ctx context.Context, limit int, handle outboxout.EventHandler) (int, error) {
	claimed := 0
	for _, event := range r.events {
		if event.PublishedAt != nil {
			continue
		}
		if claimed >= limit {
			break
		}
		claimed++
		if err := handle(ctx, event); err != nil {
			return 0, err
		}
	}

	return claimed, nil
}

func (r *fakeOutboxRepository) MarkPublished(_ context.Context, id uuid.UUID, publishedAt time.Time) error {
	if r.markPublishedErr != nil {
		return r.markPublishedErr
	}
	event := r.events[id]
	event.PublishedAt = &publishedAt
	r.events[id] = event

	return nil
}

func (r *fakeOutboxRepository) IncrementAttempts(_ context.Context, id uuid.UUID) error {
	event := r.events[id]
	event.Attempts++
	r.events[id] = event

	return nil
}

func (r *fakeOutboxRepository) FindByID(_ context.Context, id uuid.UUID) (domain.Event, error) {
	return r.events[id], nil
}

func newTestRelay(t *testing.T, repository *fakeOutboxRepository, publisher *fakePublisher) *Relay {
	t.Helper()

	relay, err := NewRelay(repository, publisher, RelayConfig{
		Interval:  time.Hour,
		BatchSize: 10,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	relay.now = func() time.Time {
		return relayTestTime().Add(time.Hour)
	}

	return relay
}

func mustRelayTestEvent(t *testing.T) domain.Event {
	t.Helper()

	event, err := domain.NewEvent(
		uuid.New(),
		domain.ReminderAggregateType,
		uuid.New(),
		domain.NotificationRequestedV1Type,
		[]byte(`{"reminder_id":"reminder"}`),
		relayTestTime(),
	)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	return event
}

func relayTestTime() time.Time {
	return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
}
