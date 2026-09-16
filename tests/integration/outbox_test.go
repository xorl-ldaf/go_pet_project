package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	outboxkafka "go_pet_project/internal/outbox/adapter/out/kafka"
	outboxpostgres "go_pet_project/internal/outbox/adapter/out/postgres"
	outboxout "go_pet_project/internal/outbox/application/port/out"
	outboxservice "go_pet_project/internal/outbox/application/service"
	outboxdomain "go_pet_project/internal/outbox/domain"
	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/database"
	reminderscheduler "go_pet_project/internal/reminder/adapter/in/scheduler"
	reminderpostgres "go_pet_project/internal/reminder/adapter/out/postgres"
	reminderout "go_pet_project/internal/reminder/application/port/out"
	reminderdomain "go_pet_project/internal/reminder/domain"
	taskpostgres "go_pet_project/internal/task/adapter/out/postgres"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestPostgresOutboxRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_outbox_repository_test_")
	repository := outboxpostgres.NewRepository(pg.GORM)
	createdAt := outboxIntegrationTime()

	oldEvent := mustOutboxEvent(t, uuid.New(), createdAt.Add(-2*time.Hour))
	newEvent := mustOutboxEvent(t, uuid.New(), createdAt.Add(-time.Hour))
	publishedEvent := mustOutboxEvent(t, uuid.New(), createdAt.Add(-90*time.Minute))
	for _, event := range []outboxdomain.Event{newEvent, oldEvent, publishedEvent} {
		if _, err := repository.Create(ctx, event); err != nil {
			t.Fatalf("create outbox event: %v", err)
		}
	}
	if err := repository.MarkPublished(ctx, publishedEvent.ID, createdAt); err != nil {
		t.Fatalf("mark published: %v", err)
	}
	if err := repository.IncrementAttempts(ctx, newEvent.ID); err != nil {
		t.Fatalf("increment attempts: %v", err)
	}
	foundNew, err := repository.FindByID(ctx, newEvent.ID)
	if err != nil {
		t.Fatalf("find event: %v", err)
	}
	if foundNew.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", foundNew.Attempts)
	}

	var claimed []uuid.UUID
	count, err := repository.ClaimUnpublished(ctx, 1, func(_ context.Context, event outboxdomain.Event) error {
		claimed = append(claimed, event.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("ClaimUnpublished limit 1: %v", err)
	}
	if count != 1 || len(claimed) != 1 || claimed[0] != oldEvent.ID {
		t.Fatalf("claimed = %v count=%d, want oldest unpublished", claimed, count)
	}

	claimed = nil
	count, err = repository.ClaimUnpublished(ctx, 10, func(_ context.Context, event outboxdomain.Event) error {
		claimed = append(claimed, event.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("ClaimUnpublished limit 10: %v", err)
	}
	if count != 2 || len(claimed) != 2 || claimed[0] != oldEvent.ID || claimed[1] != newEvent.ID {
		t.Fatalf("claimed = %v count=%d, want unpublished ordered by created_at", claimed, count)
	}
}

func TestPostgresOutboxClaimUnpublishedSkipLocked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_outbox_skip_locked_test_")
	repository := outboxpostgres.NewRepository(pg.GORM)
	first := mustOutboxEvent(t, uuid.New(), outboxIntegrationTime().Add(-time.Hour))
	second := mustOutboxEvent(t, uuid.New(), outboxIntegrationTime())
	if _, err := repository.Create(ctx, first); err != nil {
		t.Fatalf("create first event: %v", err)
	}
	if _, err := repository.Create(ctx, second); err != nil {
		t.Fatalf("create second event: %v", err)
	}

	aClaimed := make(chan uuid.UUID, 1)
	releaseA := make(chan struct{})
	aErr := make(chan error, 1)
	go func() {
		_, err := repository.ClaimUnpublished(ctx, 1, func(_ context.Context, event outboxdomain.Event) error {
			aClaimed <- event.ID
			<-releaseA
			return nil
		})
		aErr <- err
	}()

	var idA uuid.UUID
	select {
	case idA = <-aClaimed:
	case <-time.After(5 * time.Second):
		t.Fatal("relay A did not claim event")
	}

	var idsB []uuid.UUID
	bDone := make(chan error, 1)
	go func() {
		_, err := repository.ClaimUnpublished(ctx, 2, func(_ context.Context, event outboxdomain.Event) error {
			idsB = append(idsB, event.ID)
			return nil
		})
		bDone <- err
	}()

	select {
	case err := <-bDone:
		if err != nil {
			t.Fatalf("relay B ClaimUnpublished: %v", err)
		}
	case <-time.After(5 * time.Second):
		close(releaseA)
		t.Fatal("relay B blocked instead of skipping locked outbox row")
	}
	close(releaseA)
	if err := <-aErr; err != nil {
		t.Fatalf("relay A ClaimUnpublished: %v", err)
	}
	if len(idsB) != 1 {
		t.Fatalf("relay B claimed %d events, want 1: %v", len(idsB), idsB)
	}
	if idsB[0] == idA {
		t.Fatalf("same outbox row claimed by both relays: %s", idA)
	}
}

func TestReminderSchedulerTransactionalOutbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("success marks reminder sent and creates outbox row", func(t *testing.T) {
		pg, taskRepo, reminderRepo, outboxRepo, reminder := setupReminderOutboxTest(ctx, t, "todo_reminder_outbox_success_test_")
		runner := newReminderOutboxRunner(t, taskRepo, reminderRepo, reminderRepo, outboxRepo)

		processed, err := runner.ProcessOnce(ctx)
		if err != nil {
			t.Fatalf("ProcessOnce: %v", err)
		}
		if processed != 1 {
			t.Fatalf("processed = %d, want 1", processed)
		}
		foundReminder, err := reminderRepo.FindByID(ctx, reminder.ID)
		if err != nil {
			t.Fatalf("find reminder: %v", err)
		}
		if foundReminder.State != reminderdomain.StateSent || foundReminder.SentAt == nil {
			t.Fatalf("reminder state = %s sent_at=%v, want SENT with sent_at", foundReminder.State, foundReminder.SentAt)
		}
		if count := countOutboxEvents(ctx, t, pg, reminder.ID); count != 1 {
			t.Fatalf("outbox count = %d, want 1", count)
		}
	})

	t.Run("outbox insert failure leaves reminder pending", func(t *testing.T) {
		_, taskRepo, reminderRepo, _, reminder := setupReminderOutboxTest(ctx, t, "todo_reminder_outbox_insert_failure_test_")
		failingOutbox := &failingOutboxRepository{err: errForcedOutboxCreate}
		runner := newReminderOutboxRunner(t, taskRepo, reminderRepo, reminderRepo, failingOutbox)

		_, err := runner.ProcessOnce(ctx)
		if !errors.Is(err, errForcedOutboxCreate) {
			t.Fatalf("ProcessOnce error = %v, want outbox create error", err)
		}
		foundReminder, findErr := reminderRepo.FindByID(ctx, reminder.ID)
		if findErr != nil {
			t.Fatalf("find reminder: %v", findErr)
		}
		if foundReminder.State != reminderdomain.StatePending {
			t.Fatalf("reminder state = %s, want PENDING", foundReminder.State)
		}
	})

	t.Run("reminder update failure rolls back outbox row", func(t *testing.T) {
		pg, taskRepo, reminderRepo, outboxRepo, reminder := setupReminderOutboxTest(ctx, t, "todo_reminder_outbox_update_failure_test_")
		failingReminderRepo := &failingReminderRepository{Repository: reminderRepo, err: errForcedReminderUpdate}
		runner := newReminderOutboxRunner(t, taskRepo, reminderRepo, failingReminderRepo, outboxRepo)

		_, err := runner.ProcessOnce(ctx)
		if !errors.Is(err, errForcedReminderUpdate) {
			t.Fatalf("ProcessOnce error = %v, want reminder update error", err)
		}
		if count := countOutboxEvents(ctx, t, pg, reminder.ID); count != 0 {
			t.Fatalf("outbox count = %d, want 0 after rollback", count)
		}
		foundReminder, findErr := reminderRepo.FindByID(ctx, reminder.ID)
		if findErr != nil {
			t.Fatalf("find reminder: %v", findErr)
		}
		if foundReminder.State != reminderdomain.StatePending {
			t.Fatalf("reminder state = %s, want PENDING after rollback", foundReminder.State)
		}
	})
}

func TestConcurrentReminderSchedulersCreateSingleOutboxEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg, taskRepo, reminderRepo, outboxRepo, reminder := setupReminderOutboxTest(ctx, t, "todo_reminder_outbox_concurrent_test_")
	runnerA := newReminderOutboxRunner(t, taskRepo, reminderRepo, reminderRepo, outboxRepo)
	runnerB := newReminderOutboxRunner(t, taskRepo, reminderRepo, reminderRepo, outboxRepo)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, runner := range []*reminderscheduler.Runner{runnerA, runnerB} {
		wg.Add(1)
		go func(runner *reminderscheduler.Runner) {
			defer wg.Done()
			_, err := runner.ProcessOnce(ctx)
			errs <- err
		}(runner)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ProcessOnce: %v", err)
		}
	}

	if count := countOutboxEvents(ctx, t, pg, reminder.ID); count != 1 {
		t.Fatalf("outbox count = %d, want 1", count)
	}
	foundReminder, err := reminderRepo.FindByID(ctx, reminder.ID)
	if err != nil {
		t.Fatalf("find reminder: %v", err)
	}
	if foundReminder.State != reminderdomain.StateSent {
		t.Fatalf("reminder state = %s, want SENT", foundReminder.State)
	}
}

func TestOutboxRelayDoesNotPublishSameRowConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_outbox_relay_lock_test_")
	repository := outboxpostgres.NewRepository(pg.GORM)
	event := mustOutboxEvent(t, uuid.New(), outboxIntegrationTime())
	if _, err := repository.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	publisherA := &blockingOutboxPublisher{started: started, release: release}
	publisherB := &recordingOutboxPublisher{}
	relayA := newIntegrationRelay(t, repository, publisherA)
	relayB := newIntegrationRelay(t, repository, publisherB)

	errA := make(chan error, 1)
	go func() {
		_, err := relayA.ProcessOnce(ctx)
		errA <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("relay A did not start publishing")
	}

	countB, err := relayB.ProcessOnce(ctx)
	if err != nil {
		close(release)
		t.Fatalf("relay B ProcessOnce: %v", err)
	}
	if countB != 0 || publisherB.calls != 0 {
		close(release)
		t.Fatalf("relay B count=%d publisherCalls=%d, want no concurrent publish", countB, publisherB.calls)
	}
	close(release)
	if err := <-errA; err != nil {
		t.Fatalf("relay A ProcessOnce: %v", err)
	}
}

func TestKafkaOutboxRelayPublishesEnvelope(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	pg := setupTaskHTTPDatabase(ctx, t, "todo_outbox_kafka_test_")
	waitForKafka(ctx, t, cfg.Kafka)

	topic := cfg.Kafka.NotificationTopic
	repository := outboxpostgres.NewRepository(pg.GORM)
	event := mustOutboxEvent(t, uuid.New(), outboxIntegrationTime())
	if _, err := repository.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	publisher, err := outboxkafka.NewPublisher(cfg.Kafka.Brokers, topic)
	if err != nil {
		t.Fatalf("new kafka publisher: %v", err)
	}
	defer publisher.Close()
	relay := newIntegrationRelay(t, repository, publisher)

	processed, err := relay.ProcessOnce(ctx)
	if err != nil {
		t.Fatalf("relay ProcessOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}

	record := pollKafkaRecord(ctx, t, cfg.Kafka, topic, event.ID.String())
	var envelope struct {
		EventID    string          `json:"event_id"`
		EventType  string          `json:"event_type"`
		Version    int             `json:"version"`
		OccurredAt time.Time       `json:"occurred_at"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(record.Value, &envelope); err != nil {
		t.Fatalf("unmarshal kafka envelope: %v: %s", err, string(record.Value))
	}
	if envelope.EventID != event.ID.String() ||
		envelope.EventType != outboxdomain.NotificationRequestedV1Type ||
		envelope.Version != outboxdomain.EventVersion ||
		len(envelope.Payload) == 0 {
		t.Fatalf("unexpected kafka envelope: %#v", envelope)
	}
	if string(record.Key) != event.AggregateID.String() {
		t.Fatalf("record key = %s, want %s", string(record.Key), event.AggregateID.String())
	}
	found, err := repository.FindByID(ctx, event.ID)
	if err != nil {
		t.Fatalf("find event: %v", err)
	}
	if found.PublishedAt == nil {
		t.Fatalf("PublishedAt = nil, want set")
	}
}

var (
	errForcedOutboxCreate   = errors.New("forced outbox create failure")
	errForcedReminderUpdate = errors.New("forced reminder update failure")
)

type failingOutboxRepository struct {
	err error
}

func (r *failingOutboxRepository) Create(context.Context, outboxdomain.Event) (outboxdomain.Event, error) {
	return outboxdomain.Event{}, r.err
}
func (r *failingOutboxRepository) ClaimUnpublished(context.Context, int, outboxout.EventHandler) (int, error) {
	return 0, nil
}
func (r *failingOutboxRepository) MarkPublished(context.Context, uuid.UUID, time.Time) error {
	return nil
}
func (r *failingOutboxRepository) IncrementAttempts(context.Context, uuid.UUID) error {
	return nil
}
func (r *failingOutboxRepository) FindByID(context.Context, uuid.UUID) (outboxdomain.Event, error) {
	return outboxdomain.Event{}, outboxdomain.ErrEventNotFound
}

type failingReminderRepository struct {
	*reminderpostgres.Repository
	err error
}

func (r *failingReminderRepository) Update(context.Context, reminderdomain.Reminder) (reminderdomain.Reminder, error) {
	return reminderdomain.Reminder{}, r.err
}

type blockingOutboxPublisher struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingOutboxPublisher) Publish(ctx context.Context, _ outboxdomain.Event) error {
	close(p.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.release:
		return nil
	}
}

type recordingOutboxPublisher struct {
	calls int
}

func (p *recordingOutboxPublisher) Publish(context.Context, outboxdomain.Event) error {
	p.calls++
	return nil
}

func setupReminderOutboxTest(
	ctx context.Context,
	t *testing.T,
	prefix string,
) (*database.Postgres, *taskpostgres.Repository, *reminderpostgres.Repository, *outboxpostgres.Repository, reminderdomain.Reminder) {
	t.Helper()

	pg := setupTaskHTTPDatabase(ctx, t, prefix)
	users := userpostgres.NewRepository(pg.GORM)
	taskRepo := taskpostgres.NewRepository(pg.GORM)
	reminderRepo := reminderpostgres.NewRepository(pg.GORM)
	outboxRepo := outboxpostgres.NewRepository(pg.GORM)
	creator := createTaskUser(ctx, t, users, "outbox-creator-"+uuid.NewString()+"@example.com", "outbox_creator_"+uuid.NewString()[:8])
	assignee := createTaskUser(ctx, t, users, "outbox-assignee-"+uuid.NewString()+"@example.com", "outbox_assignee_"+uuid.NewString()[:8])
	task := createRepositoryTask(ctx, t, taskRepo, creator.ID, assignee.ID, "outbox reminder task", outboxIntegrationTime().Add(-24*time.Hour))
	reminderTime := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	reminder := mustSchedulerReminder(t, task.ID, reminderTime, reminderTime.Add(-24*time.Hour), reminderdomain.StatePending)
	if _, err := reminderRepo.Create(ctx, reminder); err != nil {
		t.Fatalf("create reminder: %v", err)
	}

	return pg, taskRepo, reminderRepo, outboxRepo, reminder
}

func newReminderOutboxRunner(
	t *testing.T,
	taskRepo *taskpostgres.Repository,
	claimer *reminderpostgres.Repository,
	reminderRepo reminderout.ReminderRepository,
	outboxRepo outboxout.Repository,
) *reminderscheduler.Runner {
	t.Helper()

	processor, err := reminderscheduler.NewNotificationProcessor(taskRepo, reminderRepo, outboxRepo)
	if err != nil {
		t.Fatalf("NewNotificationProcessor: %v", err)
	}
	runner, err := reminderscheduler.NewRunner(claimer, processor, reminderscheduler.Config{
		Interval:  time.Hour,
		BatchSize: 10,
		Workers:   1,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	return runner
}

func newIntegrationRelay(t *testing.T, repository *outboxpostgres.Repository, publisher interface {
	Publish(context.Context, outboxdomain.Event) error
}) *outboxservice.Relay {
	t.Helper()

	relay, err := outboxservice.NewRelay(repository, publisher, outboxservice.RelayConfig{
		Interval:  time.Hour,
		BatchSize: 10,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}

	return relay
}

func countOutboxEvents(ctx context.Context, t *testing.T, pg *database.Postgres, aggregateID uuid.UUID) int {
	t.Helper()

	var count int
	if err := pg.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = $1`, aggregateID).Scan(&count); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}

	return count
}

func mustOutboxEvent(t *testing.T, aggregateID uuid.UUID, createdAt time.Time) outboxdomain.Event {
	t.Helper()

	payload := []byte(`{"reminder_id":"` + aggregateID.String() + `"}`)
	event, err := outboxdomain.NewEvent(uuid.New(), outboxdomain.ReminderAggregateType, aggregateID, outboxdomain.NotificationRequestedV1Type, payload, createdAt)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	return event
}

func waitForKafka(ctx context.Context, t *testing.T, cfg config.KafkaConfig) {
	t.Helper()

	deadline := time.Now().Add(90 * time.Second)
	for {
		client, err := kgo.NewClient(kgo.SeedBrokers(cfg.Brokers...))
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = client.Ping(pingCtx)
			cancel()
			client.Close()
			if err == nil {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("kafka did not become ready: %v", err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait kafka: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func pollKafkaRecord(ctx context.Context, t *testing.T, cfg config.KafkaConfig, topic string, eventID string) *kgo.Record {
	t.Helper()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{
			topic: {0: kgo.NewOffset().AtStart()},
		}),
	)
	if err != nil {
		t.Fatalf("new kafka consumer: %v", err)
	}
	defer client.Close()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		fetches := client.PollFetches(pollCtx)
		cancel()
		if err := fetches.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("poll kafka: %v", err)
		}
		for _, record := range fetches.Records() {
			var envelope struct {
				EventID string `json:"event_id"`
			}
			if err := json.Unmarshal(record.Value, &envelope); err == nil && envelope.EventID == eventID {
				return record
			}
		}
	}

	t.Fatalf("kafka record for event %s not found", eventID)
	return nil
}

func outboxIntegrationTime() time.Time {
	return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
}
