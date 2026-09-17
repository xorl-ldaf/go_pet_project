package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go_pet_project/internal/notification/application/command"
	notificationout "go_pet_project/internal/notification/application/port/out"
	"go_pet_project/internal/notification/domain"

	"github.com/google/uuid"
)

func TestTelegramLinkTokenCreateStoresHashOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakes := newServiceFakes(now)
	service := newTestNotificationService(t, fakes)

	result, err := service.CreateTelegramLinkToken(ctx, command.CreateTelegramLinkCommand{ActorID: fakes.userID})
	if err != nil {
		t.Fatalf("CreateTelegramLinkToken: %v", err)
	}
	if result.Token == "" {
		t.Fatal("token is empty")
	}
	if result.ExpiresAt != now.Add(telegramLinkTokenTTL) {
		t.Fatalf("expires_at = %s, want %s", result.ExpiresAt, now.Add(telegramLinkTokenTTL))
	}
	if len(fakes.tokens.tokens) != 1 {
		t.Fatalf("stored tokens = %d, want 1", len(fakes.tokens.tokens))
	}
	for hash := range fakes.tokens.tokens {
		if hash == result.Token {
			t.Fatal("raw token was stored as token_hash")
		}
		if len(hash) != 64 {
			t.Fatalf("token_hash length = %d, want sha256 hex length 64", len(hash))
		}
	}
}

func TestTelegramLinkTokenConsumeValidExpiredUsedAndConcurrent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakes := newServiceFakes(now)
	service := newTestNotificationService(t, fakes)

	result, err := service.CreateTelegramLinkToken(ctx, command.CreateTelegramLinkCommand{ActorID: fakes.userID})
	if err != nil {
		t.Fatalf("CreateTelegramLinkToken: %v", err)
	}
	if err := service.HandleTelegramStart(ctx, command.HandleTelegramStartCommand{Token: result.Token, ChatID: 101}); err != nil {
		t.Fatalf("HandleTelegramStart valid token: %v", err)
	}
	if _, err := fakes.links.FindByUserID(ctx, fakes.userID); err != nil {
		t.Fatalf("FindByUserID after link: %v", err)
	}
	if err := service.HandleTelegramStart(ctx, command.HandleTelegramStartCommand{Token: result.Token, ChatID: 102}); !errors.Is(err, domain.ErrTelegramLinkTokenInvalidOrExpired) {
		t.Fatalf("HandleTelegramStart used token error = %v, want invalid/expired", err)
	}

	expired, err := service.CreateTelegramLinkToken(ctx, command.CreateTelegramLinkCommand{ActorID: fakes.userID})
	if err != nil {
		t.Fatalf("CreateTelegramLinkToken expired fixture: %v", err)
	}
	service.now = func() time.Time { return now.Add(telegramLinkTokenTTL + time.Second) }
	if err := service.HandleTelegramStart(ctx, command.HandleTelegramStartCommand{Token: expired.Token, ChatID: 103}); !errors.Is(err, domain.ErrTelegramLinkTokenInvalidOrExpired) {
		t.Fatalf("HandleTelegramStart expired token error = %v, want invalid/expired", err)
	}

	service.now = func() time.Time { return now }
	concurrent, err := service.CreateTelegramLinkToken(ctx, command.CreateTelegramLinkCommand{ActorID: fakes.userID})
	if err != nil {
		t.Fatalf("CreateTelegramLinkToken concurrent fixture: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- service.HandleTelegramStart(ctx, command.HandleTelegramStartCommand{Token: concurrent.Token, ChatID: 200})
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, domain.ErrTelegramLinkTokenInvalidOrExpired) {
			failures++
			continue
		}
		t.Fatalf("unexpected concurrent consume error: %v", err)
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent consume successes=%d failures=%d, want 1/1", successes, failures)
	}
}

func TestTelegramStartRelinkUpdatesChat(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakes := newServiceFakes(now)
	service := newTestNotificationService(t, fakes)

	first, err := service.CreateTelegramLinkToken(ctx, command.CreateTelegramLinkCommand{ActorID: fakes.userID})
	if err != nil {
		t.Fatalf("Create first token: %v", err)
	}
	if err := service.HandleTelegramStart(ctx, command.HandleTelegramStartCommand{Token: first.Token, ChatID: 101}); err != nil {
		t.Fatalf("Handle first start: %v", err)
	}

	second, err := service.CreateTelegramLinkToken(ctx, command.CreateTelegramLinkCommand{ActorID: fakes.userID})
	if err != nil {
		t.Fatalf("Create second token: %v", err)
	}
	username := "new_user"
	if err := service.HandleTelegramStart(ctx, command.HandleTelegramStartCommand{Token: second.Token, ChatID: 202, TelegramUsername: &username}); err != nil {
		t.Fatalf("Handle relink start: %v", err)
	}

	link, err := fakes.links.FindByUserID(ctx, fakes.userID)
	if err != nil {
		t.Fatalf("Find link: %v", err)
	}
	if link.ChatID != 202 || link.TelegramUsername == nil || *link.TelegramUsername != "new_user" || !link.Enabled {
		t.Fatalf("link after relink = %+v, want updated chat/username/enabled", link)
	}
}

func TestNotificationEventCreatesDeliveryAtomicallyAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakes := newServiceFakes(now)
	service := newTestNotificationService(t, fakes)
	link, err := domain.NewTelegramLink(fakes.userID, 101, nil, now)
	if err != nil {
		t.Fatalf("NewTelegramLink: %v", err)
	}
	if _, err := fakes.links.Upsert(ctx, link); err != nil {
		t.Fatalf("Upsert link: %v", err)
	}

	eventID := uuid.New()
	cmd := command.HandleNotificationRequestedCommand{
		EventID:         eventID,
		RecipientUserID: fakes.userID,
		TaskID:          uuid.New(),
		ReminderID:      uuid.New(),
		OccurredAt:      now,
	}
	if err := service.HandleNotificationRequested(ctx, cmd); err != nil {
		t.Fatalf("HandleNotificationRequested first: %v", err)
	}
	if err := service.HandleNotificationRequested(ctx, cmd); err != nil {
		t.Fatalf("HandleNotificationRequested duplicate: %v", err)
	}
	if got := len(fakes.notifications.items); got != 1 {
		t.Fatalf("notifications = %d, want 1", got)
	}
	if got := len(fakes.processed.events); got != 1 {
		t.Fatalf("processed events = %d, want 1", got)
	}
	if got := len(fakes.deliveries.items); got != 1 {
		t.Fatalf("deliveries = %d, want 1", got)
	}
}

func TestNotificationAndProcessedEventRollbackWhenDeliveryCreateFails(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakes := newServiceFakes(now)
	service := newTestNotificationService(t, fakes)
	link, err := domain.NewTelegramLink(fakes.userID, 101, nil, now)
	if err != nil {
		t.Fatalf("NewTelegramLink: %v", err)
	}
	if _, err := fakes.links.Upsert(ctx, link); err != nil {
		t.Fatalf("Upsert link: %v", err)
	}
	fakes.deliveries.failCreate = true
	fakes.tx.rollback = func() {
		fakes.notifications.items = map[uuid.UUID]domain.Notification{}
		fakes.processed.events = map[string]bool{}
		fakes.deliveries.items = map[uuid.UUID]domain.NotificationDelivery{}
	}

	err = service.HandleNotificationRequested(ctx, command.HandleNotificationRequestedCommand{
		EventID:         uuid.New(),
		RecipientUserID: fakes.userID,
		TaskID:          uuid.New(),
		ReminderID:      uuid.New(),
		OccurredAt:      now,
	})
	if err == nil {
		t.Fatal("HandleNotificationRequested error = nil, want delivery failure")
	}
	if len(fakes.notifications.items) != 0 || len(fakes.processed.events) != 0 || len(fakes.deliveries.items) != 0 {
		t.Fatalf("transaction state was not rolled back: notifications=%d processed=%d deliveries=%d", len(fakes.notifications.items), len(fakes.processed.events), len(fakes.deliveries.items))
	}
}

func TestDeliveryRunnerRetryPolicy(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakes := newServiceFakes(now)
	notification := mustNotification(t, fakes.userID, now)
	fakes.notifications.items[notification.ID] = notification
	link, err := domain.NewTelegramLink(fakes.userID, 101, nil, now)
	if err != nil {
		t.Fatalf("NewTelegramLink: %v", err)
	}
	fakes.links.links[fakes.userID] = link
	delivery, err := domain.NewNotificationDelivery(uuid.New(), notification.ID, domain.DeliveryChannelTelegram, now)
	if err != nil {
		t.Fatalf("NewNotificationDelivery: %v", err)
	}
	fakes.deliveries.items[delivery.ID] = delivery

	sender := &fakeSender{failures: 4}
	runner := newTestDeliveryRunner(t, fakes, sender)
	runner.now = func() time.Time { return now }

	if err := runner.ProcessDue(ctx); err != nil {
		t.Fatalf("ProcessDue failure #1: %v", err)
	}
	assertDelivery(t, fakes.deliveries.items[delivery.ID], domain.DeliveryStatusPending, 1, now.Add(10*time.Second))

	now = now.Add(10 * time.Second)
	if err := runner.ProcessDue(ctx); err != nil {
		t.Fatalf("ProcessDue failure #2: %v", err)
	}
	assertDelivery(t, fakes.deliveries.items[delivery.ID], domain.DeliveryStatusPending, 2, now.Add(time.Minute))

	now = now.Add(time.Minute)
	if err := runner.ProcessDue(ctx); err != nil {
		t.Fatalf("ProcessDue failure #3: %v", err)
	}
	assertDelivery(t, fakes.deliveries.items[delivery.ID], domain.DeliveryStatusPending, 3, now.Add(5*time.Minute))

	now = now.Add(5 * time.Minute)
	if err := runner.ProcessDue(ctx); err != nil {
		t.Fatalf("ProcessDue final failure: %v", err)
	}
	got := fakes.deliveries.items[delivery.ID]
	if got.Status != domain.DeliveryStatusFailed || got.Attempts != 4 || got.NextAttemptAt != nil {
		t.Fatalf("final delivery = %+v, want FAILED attempts=4 next=nil", got)
	}
}

func TestDeliveryRunnerSuccessFirstAttempt(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakes := newServiceFakes(now)
	notification := mustNotification(t, fakes.userID, now)
	fakes.notifications.items[notification.ID] = notification
	link, err := domain.NewTelegramLink(fakes.userID, 101, nil, now)
	if err != nil {
		t.Fatalf("NewTelegramLink: %v", err)
	}
	fakes.links.links[fakes.userID] = link
	delivery, err := domain.NewNotificationDelivery(uuid.New(), notification.ID, domain.DeliveryChannelTelegram, now)
	if err != nil {
		t.Fatalf("NewNotificationDelivery: %v", err)
	}
	fakes.deliveries.items[delivery.ID] = delivery

	runner := newTestDeliveryRunner(t, fakes, &fakeSender{})
	runner.now = func() time.Time { return now }
	if err := runner.ProcessDue(ctx); err != nil {
		t.Fatalf("ProcessDue: %v", err)
	}

	got := fakes.deliveries.items[delivery.ID]
	if got.Status != domain.DeliveryStatusSent || got.Attempts != 1 || got.SentAt == nil || got.NextAttemptAt != nil || got.LastError != nil {
		t.Fatalf("delivery after success = %+v, want SENT attempts=1 sent_at set", got)
	}
}

func assertDelivery(t *testing.T, delivery domain.NotificationDelivery, status domain.DeliveryStatus, attempts int, next time.Time) {
	t.Helper()
	if delivery.Status != status || delivery.Attempts != attempts || delivery.NextAttemptAt == nil || !delivery.NextAttemptAt.Equal(next) {
		t.Fatalf("delivery = %+v, want status=%s attempts=%d next=%s", delivery, status, attempts, next)
	}
	if delivery.LastError == nil || *delivery.LastError == "" {
		t.Fatal("last_error is empty after failure")
	}
}

type serviceFakes struct {
	userID        uuid.UUID
	notifications *fakeNotificationRepo
	processed     *fakeProcessedRepo
	deliveries    *fakeDeliveryRepo
	links         *fakeLinkRepo
	tokens        *fakeTokenRepo
	tx            *fakeTxRunner
}

func newServiceFakes(now time.Time) *serviceFakes {
	return &serviceFakes{
		userID:        uuid.New(),
		notifications: &fakeNotificationRepo{items: map[uuid.UUID]domain.Notification{}},
		processed:     &fakeProcessedRepo{events: map[string]bool{}},
		deliveries:    &fakeDeliveryRepo{items: map[uuid.UUID]domain.NotificationDelivery{}, now: now},
		links:         &fakeLinkRepo{links: map[uuid.UUID]domain.TelegramLink{}},
		tokens:        &fakeTokenRepo{tokens: map[string]domain.TelegramLinkToken{}},
		tx:            &fakeTxRunner{},
	}
}

func newTestNotificationService(t *testing.T, f *serviceFakes) *NotificationService {
	t.Helper()
	service, err := NewNotificationService(
		f.notifications,
		f.processed,
		f.tx,
		ConsumerName,
		WithTelegramRepositories(f.deliveries, f.links, f.tokens),
	)
	if err != nil {
		t.Fatalf("NewNotificationService: %v", err)
	}
	now := f.deliveries.now
	service.now = func() time.Time { return now }

	return service
}

func newTestDeliveryRunner(t *testing.T, f *serviceFakes, sender notificationout.MessageSender) *DeliveryRunner {
	t.Helper()
	runner, err := NewDeliveryRunner(
		f.notifications,
		f.deliveries,
		f.links,
		f.tx,
		sender,
		[]time.Duration{10 * time.Second, time.Minute, 5 * time.Minute},
		time.Second,
		10,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDeliveryRunner: %v", err)
	}

	return runner
}

func mustNotification(t *testing.T, userID uuid.UUID, now time.Time) domain.Notification {
	t.Helper()
	notification, err := domain.NewNotification(uuid.New(), userID, nil, nil, domain.TypeTaskReminder, "Task reminder", "A task reminder is due.", now)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}

	return notification
}

type fakeTxRunner struct {
	rollback func()
}

func (r *fakeTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	err := fn(ctx)
	if err != nil && r.rollback != nil {
		r.rollback()
	}

	return err
}

type fakeNotificationRepo struct {
	items map[uuid.UUID]domain.Notification
}

func (r *fakeNotificationRepo) Create(_ context.Context, notification domain.Notification) (domain.Notification, error) {
	r.items[notification.ID] = notification
	return notification, nil
}

func (r *fakeNotificationRepo) FindByID(_ context.Context, id uuid.UUID) (domain.Notification, error) {
	notification, ok := r.items[id]
	if !ok {
		return domain.Notification{}, domain.ErrNotificationNotFound
	}
	return notification, nil
}

func (r *fakeNotificationRepo) ListByUserID(_ context.Context, userID uuid.UUID) ([]domain.Notification, error) {
	var notifications []domain.Notification
	for _, notification := range r.items {
		if notification.UserID == userID {
			notifications = append(notifications, notification)
		}
	}
	return notifications, nil
}

func (r *fakeNotificationRepo) CountUnreadByUserID(_ context.Context, userID uuid.UUID) (int, error) {
	count := 0
	for _, notification := range r.items {
		if notification.UserID == userID && notification.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (r *fakeNotificationRepo) MarkRead(_ context.Context, id uuid.UUID, readAt time.Time) error {
	notification, ok := r.items[id]
	if !ok {
		return domain.ErrNotificationNotFound
	}
	notification.ReadAt = &readAt
	r.items[id] = notification
	return nil
}

func (r *fakeNotificationRepo) MarkAllRead(_ context.Context, userID uuid.UUID, readAt time.Time) error {
	for id, notification := range r.items {
		if notification.UserID == userID {
			notification.ReadAt = &readAt
			r.items[id] = notification
		}
	}
	return nil
}

type fakeProcessedRepo struct {
	events map[string]bool
}

func (r *fakeProcessedRepo) TryInsert(_ context.Context, consumerName string, eventID uuid.UUID, _ time.Time) (bool, error) {
	key := consumerName + ":" + eventID.String()
	if r.events[key] {
		return false, nil
	}
	r.events[key] = true
	return true, nil
}

func (r *fakeProcessedRepo) Exists(_ context.Context, consumerName string, eventID uuid.UUID) (bool, error) {
	return r.events[consumerName+":"+eventID.String()], nil
}

type fakeDeliveryRepo struct {
	items      map[uuid.UUID]domain.NotificationDelivery
	now        time.Time
	failCreate bool
}

func (r *fakeDeliveryRepo) Create(_ context.Context, delivery domain.NotificationDelivery) (domain.NotificationDelivery, error) {
	if r.failCreate {
		return domain.NotificationDelivery{}, errors.New("forced delivery insert failure")
	}
	r.items[delivery.ID] = delivery
	return delivery, nil
}

func (r *fakeDeliveryRepo) ClaimDue(_ context.Context, now time.Time, limit int) ([]domain.NotificationDelivery, error) {
	var due []domain.NotificationDelivery
	for _, delivery := range r.items {
		if len(due) == limit {
			break
		}
		if delivery.Status == domain.DeliveryStatusPending && delivery.NextAttemptAt != nil && !delivery.NextAttemptAt.After(now) {
			due = append(due, delivery)
		}
	}
	return due, nil
}

func (r *fakeDeliveryRepo) MarkSent(_ context.Context, id uuid.UUID, attempts int, sentAt time.Time) error {
	delivery := r.items[id]
	delivery.Status = domain.DeliveryStatusSent
	delivery.Attempts = attempts
	delivery.SentAt = &sentAt
	delivery.NextAttemptAt = nil
	delivery.LastError = nil
	r.items[id] = delivery
	return nil
}

func (r *fakeDeliveryRepo) ScheduleRetry(_ context.Context, id uuid.UUID, attempts int, nextAttemptAt time.Time, lastError string) error {
	delivery := r.items[id]
	delivery.Status = domain.DeliveryStatusPending
	delivery.Attempts = attempts
	delivery.NextAttemptAt = &nextAttemptAt
	delivery.LastError = &lastError
	r.items[id] = delivery
	return nil
}

func (r *fakeDeliveryRepo) MarkFailed(_ context.Context, id uuid.UUID, attempts int, lastError string) error {
	delivery := r.items[id]
	delivery.Status = domain.DeliveryStatusFailed
	delivery.Attempts = attempts
	delivery.NextAttemptAt = nil
	delivery.LastError = &lastError
	r.items[id] = delivery
	return nil
}

type fakeLinkRepo struct {
	links map[uuid.UUID]domain.TelegramLink
}

func (r *fakeLinkRepo) FindByUserID(_ context.Context, userID uuid.UUID) (domain.TelegramLink, error) {
	link, ok := r.links[userID]
	if !ok {
		return domain.TelegramLink{}, domain.ErrTelegramLinkNotFound
	}
	return link, nil
}

func (r *fakeLinkRepo) Upsert(_ context.Context, link domain.TelegramLink) (domain.TelegramLink, error) {
	r.links[link.UserID] = link
	return link, nil
}

func (r *fakeLinkRepo) SetEnabled(_ context.Context, userID uuid.UUID, enabled bool) error {
	link, ok := r.links[userID]
	if !ok {
		return domain.ErrTelegramLinkNotFound
	}
	link.Enabled = enabled
	r.links[userID] = link
	return nil
}

func (r *fakeLinkRepo) Delete(_ context.Context, userID uuid.UUID) error {
	delete(r.links, userID)
	return nil
}

type fakeTokenRepo struct {
	mu     sync.Mutex
	tokens map[string]domain.TelegramLinkToken
}

func (r *fakeTokenRepo) Create(_ context.Context, token domain.TelegramLinkToken) (domain.TelegramLinkToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token.TokenHash] = token
	return token, nil
}

func (r *fakeTokenRepo) ConsumeByHash(_ context.Context, tokenHash string, usedAt time.Time) (domain.TelegramLinkToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenHash]
	if !ok || token.UsedAt != nil || !token.ExpiresAt.After(usedAt) {
		return domain.TelegramLinkToken{}, domain.ErrTelegramLinkTokenNotFound
	}
	token.UsedAt = &usedAt
	r.tokens[tokenHash] = token
	return token, nil
}

type fakeSender struct {
	mu       sync.Mutex
	failures int
	sends    int
}

func (s *fakeSender) Send(_ context.Context, _ notificationout.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sends++
	if s.failures > 0 {
		s.failures--
		return errors.New("send failed")
	}
	return nil
}
