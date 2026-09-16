package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	notificationhttp "go_pet_project/internal/notification/adapter/in/http"
	notificationkafka "go_pet_project/internal/notification/adapter/in/kafka"
	notificationpostgres "go_pet_project/internal/notification/adapter/out/postgres"
	notificationcmd "go_pet_project/internal/notification/application/command"
	notificationservice "go_pet_project/internal/notification/application/service"
	notificationdomain "go_pet_project/internal/notification/domain"
	outboxkafka "go_pet_project/internal/outbox/adapter/out/kafka"
	outboxpostgres "go_pet_project/internal/outbox/adapter/out/postgres"
	outboxservice "go_pet_project/internal/outbox/application/service"
	outboxdomain "go_pet_project/internal/outbox/domain"
	"go_pet_project/internal/platform/database"
	reminderscheduler "go_pet_project/internal/reminder/adapter/in/scheduler"
	reminderpostgres "go_pet_project/internal/reminder/adapter/out/postgres"
	reminderdomain "go_pet_project/internal/reminder/domain"
	taskpostgres "go_pet_project/internal/task/adapter/out/postgres"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"

	"github.com/google/uuid"
)

func TestPostgresNotificationRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg, userID, taskID, reminderID := setupNotificationDB(ctx, t, "todo_notification_repository_test_")
	repo := notificationpostgres.NewNotificationRepository(pg.GORM)
	otherUserID := createTaskUser(ctx, t, userpostgres.NewRepository(pg.GORM), "notification-other@example.com", "notification_other").ID

	oldNotification := mustNotification(t, userID, taskID, reminderID, notificationTestTime())
	newNotification := mustNotification(t, userID, taskID, reminderID, notificationTestTime().Add(time.Hour))
	otherNotification := mustNotification(t, otherUserID, taskID, reminderID, notificationTestTime().Add(2*time.Hour))
	for _, notification := range []notificationdomain.Notification{oldNotification, newNotification, otherNotification} {
		if _, err := repo.Create(ctx, notification); err != nil {
			t.Fatalf("create notification: %v", err)
		}
	}

	list, err := repo.ListByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(list) != 2 || list[0].ID != newNotification.ID || list[1].ID != oldNotification.ID {
		t.Fatalf("list mismatch: %#v", list)
	}
	count, err := repo.CountUnreadByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("CountUnreadByUserID: %v", err)
	}
	if count != 2 {
		t.Fatalf("unread count = %d, want 2", count)
	}
	if err := repo.MarkRead(ctx, oldNotification.ID, notificationTestTime().Add(3*time.Hour)); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	count, _ = repo.CountUnreadByUserID(ctx, userID)
	if count != 1 {
		t.Fatalf("unread count after mark read = %d, want 1", count)
	}
	if err := repo.MarkAllRead(ctx, userID, notificationTestTime().Add(4*time.Hour)); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	count, _ = repo.CountUnreadByUserID(ctx, userID)
	if count != 0 {
		t.Fatalf("unread count after read all = %d, want 0", count)
	}
}

func TestNotificationServiceIdempotency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg, userID, taskID, reminderID := setupNotificationDB(ctx, t, "todo_notification_idempotency_test_")
	service := newIntegrationNotificationService(t, pg, "idempotency-test")
	eventID := uuid.New()
	cmd := notificationcmd.HandleNotificationRequestedCommand{
		EventID:         eventID,
		RecipientUserID: userID,
		TaskID:          taskID,
		ReminderID:      reminderID,
		OccurredAt:      notificationTestTime(),
	}

	if err := service.HandleNotificationRequested(ctx, cmd); err != nil {
		t.Fatalf("HandleNotificationRequested: %v", err)
	}
	if err := service.HandleNotificationRequested(ctx, cmd); err != nil {
		t.Fatalf("HandleNotificationRequested duplicate: %v", err)
	}
	if count := countNotifications(ctx, t, pg, userID); count != 1 {
		t.Fatalf("notification count = %d, want 1", count)
	}
	if exists := processedEventExists(ctx, t, pg, "idempotency-test", eventID); !exists {
		t.Fatalf("processed event missing")
	}
}

func TestNotificationServiceInsertFailureRollsBackProcessedEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg, _, taskID, reminderID := setupNotificationDB(ctx, t, "todo_notification_failure_test_")
	service := newIntegrationNotificationService(t, pg, "failure-test")
	eventID := uuid.New()
	err := service.HandleNotificationRequested(ctx, notificationcmd.HandleNotificationRequestedCommand{
		EventID:         eventID,
		RecipientUserID: uuid.New(),
		TaskID:          taskID,
		ReminderID:      reminderID,
		OccurredAt:      notificationTestTime(),
	})
	if err == nil {
		t.Fatal("HandleNotificationRequested error = nil, want FK failure")
	}
	if exists := processedEventExists(ctx, t, pg, "failure-test", eventID); exists {
		t.Fatalf("processed event exists after notification insert rollback")
	}
}

func TestNotificationServiceConcurrentDuplicate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg, userID, taskID, reminderID := setupNotificationDB(ctx, t, "todo_notification_concurrent_test_")
	service := newIntegrationNotificationService(t, pg, "concurrent-test")
	eventID := uuid.New()
	cmd := notificationcmd.HandleNotificationRequestedCommand{
		EventID:         eventID,
		RecipientUserID: userID,
		TaskID:          taskID,
		ReminderID:      reminderID,
		OccurredAt:      notificationTestTime(),
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- service.HandleNotificationRequested(ctx, cmd)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("HandleNotificationRequested concurrent: %v", err)
		}
	}
	if count := countNotifications(ctx, t, pg, userID); count != 1 {
		t.Fatalf("notification count = %d, want 1", count)
	}
	if count := countProcessedEvents(ctx, t, pg, "concurrent-test", eventID); count != 1 {
		t.Fatalf("processed event count = %d, want 1", count)
	}
}

func TestNotificationKafkaConsumerIdempotency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	pg, userID, taskID, reminderID := setupNotificationDB(ctx, t, "todo_notification_kafka_test_")
	waitForKafka(ctx, t, cfg.Kafka)
	topic := uniqueKafkaTopic(cfg.Kafka.NotificationTopic, "notification-kafka")
	group := "notification-kafka-test-" + uuid.NewString()
	service := newIntegrationNotificationService(t, pg, group)
	publisher, err := outboxkafka.NewPublisher(cfg.Kafka.Brokers, topic)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer publisher.Close()
	event := mustNotificationOutboxEvent(t, uuid.New(), userID, taskID, reminderID)
	if err := publisher.Publish(ctx, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	consumer, err := notificationkafka.NewConsumer(cfg.Kafka.Brokers, topic, group, service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer stopConsumer()
	defer consumer.Close()
	go func() {
		_ = consumer.Run(consumerCtx)
	}()

	waitForNotificationCount(ctx, t, pg, userID, 1)
	if err := publisher.Publish(ctx, event); err != nil {
		t.Fatalf("publish duplicate event: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if count := countNotifications(ctx, t, pg, userID); count != 1 {
		t.Fatalf("notification count after duplicate = %d, want 1", count)
	}
}

func TestNotificationHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := setupTaskHTTPDatabase(ctx, t, "todo_notification_http_test_")
	router, _ := buildTaskHTTPRouter(t, pg)
	tokens := registerAndLoginTaskUser(t, router, "notification-http@example.com", "notification_http", "plain-password")
	otherTokens := registerAndLoginTaskUser(t, router, "notification-http-other@example.com", "notification_http_other", "plain-password")
	user := currentTaskHTTPUser(t, router, tokens.AccessToken)
	otherUser := currentTaskHTTPUser(t, router, otherTokens.AccessToken)
	userID := uuid.MustParse(user.ID)
	otherUserID := uuid.MustParse(otherUser.ID)
	taskID := createTaskForNotificationHTTP(ctx, t, pg, userID)
	reminderID := createReminderForNotificationHTTP(ctx, t, pg, taskID)
	repo := notificationpostgres.NewNotificationRepository(pg.GORM)
	notification := mustNotification(t, userID, taskID, reminderID, notificationTestTime())
	created, err := repo.Create(ctx, notification)
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	otherNotification := mustNotification(t, otherUserID, taskID, reminderID, notificationTestTime())
	if _, err := repo.Create(ctx, otherNotification); err != nil {
		t.Fatalf("create other notification: %v", err)
	}

	noAuth := request(t, router, http.MethodGet, "/api/v1/notifications", nil, nil)
	if noAuth.Code != http.StatusUnauthorized {
		t.Fatalf("no auth status = %d, want 401", noAuth.Code)
	}
	list := authenticatedRequest(t, router, http.MethodGet, "/api/v1/notifications", nil, tokens.AccessToken)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", list.Code, list.Body.String())
	}
	if len(decodeNotificationHTTPList(t, list).Items) != 1 {
		t.Fatalf("list response mismatch: %s", list.Body.String())
	}
	count := authenticatedRequest(t, router, http.MethodGet, "/api/v1/notifications/unread-count", nil, tokens.AccessToken)
	if count.Code != http.StatusOK || decodeUnreadCount(t, count).Count != 1 {
		t.Fatalf("count response = %d %s, want count 1", count.Code, count.Body.String())
	}
	invalid := authenticatedRequest(t, router, http.MethodPost, "/api/v1/notifications/not-a-uuid/read", nil, tokens.AccessToken)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid mark status = %d, want 400", invalid.Code)
	}
	foreign := authenticatedRequest(t, router, http.MethodPost, "/api/v1/notifications/"+created.ID.String()+"/read", nil, otherTokens.AccessToken)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign mark status = %d, want 404", foreign.Code)
	}
	mark := authenticatedRequest(t, router, http.MethodPost, "/api/v1/notifications/"+created.ID.String()+"/read", nil, tokens.AccessToken)
	if mark.Code != http.StatusNoContent {
		t.Fatalf("mark status = %d, want 204: %s", mark.Code, mark.Body.String())
	}
	count = authenticatedRequest(t, router, http.MethodGet, "/api/v1/notifications/unread-count", nil, tokens.AccessToken)
	if decodeUnreadCount(t, count).Count != 0 {
		t.Fatalf("count after read = %s, want 0", count.Body.String())
	}
	if _, err := repo.Create(ctx, mustNotification(t, userID, taskID, reminderID, notificationTestTime().Add(time.Hour))); err != nil {
		t.Fatalf("create second notification: %v", err)
	}
	readAll := authenticatedRequest(t, router, http.MethodPost, "/api/v1/notifications/read-all", nil, tokens.AccessToken)
	if readAll.Code != http.StatusNoContent {
		t.Fatalf("read all status = %d, want 204", readAll.Code)
	}
	count = authenticatedRequest(t, router, http.MethodGet, "/api/v1/notifications/unread-count", nil, tokens.AccessToken)
	if decodeUnreadCount(t, count).Count != 0 {
		t.Fatalf("count after read all = %s, want 0", count.Body.String())
	}
}

func TestReminderNotificationE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	pg := setupTaskHTTPDatabase(ctx, t, "todo_notification_e2e_test_")
	router, _ := buildTaskHTTPRouter(t, pg)
	waitForKafka(ctx, t, cfg.Kafka)
	topic := uniqueKafkaTopic(cfg.Kafka.NotificationTopic, "notification-e2e")
	creatorTokens := registerAndLoginTaskUser(t, router, "notification-e2e-creator@example.com", "notification_e2e_creator", "plain-password")
	assigneeTokens := registerAndLoginTaskUser(t, router, "notification-e2e-assignee@example.com", "notification_e2e_assignee", "plain-password")
	creator := currentTaskHTTPUser(t, router, creatorTokens.AccessToken)
	assignee := currentTaskHTTPUser(t, router, assigneeTokens.AccessToken)
	insertAssignmentPermission(ctx, t, pg.SQL, creator.ID, assignee.ID)

	deadline := time.Now().UTC().Add(24 * time.Hour)
	createTask := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks", map[string]any{
		"assignee_id": assignee.ID,
		"title":       "Reminder notification e2e",
		"deadline_at": deadline.Format(time.RFC3339),
	}, creatorTokens.AccessToken)
	if createTask.Code != http.StatusCreated {
		t.Fatalf("create task status = %d: %s", createTask.Code, createTask.Body.String())
	}
	task := decodeTaskHTTPResponse(t, createTask)
	createReminder := authenticatedJSONRequest(t, router, http.MethodPost, "/api/v1/tasks/"+task.ID+"/reminders", map[string]any{
		"kind":       string(reminderdomain.KindAbsolute),
		"trigger_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}, creatorTokens.AccessToken)
	if createReminder.Code != http.StatusCreated {
		t.Fatalf("create reminder status = %d: %s", createReminder.Code, createReminder.Body.String())
	}
	reminder := decodeReminderHTTPResponse(t, createReminder)
	_, err := pg.SQL.ExecContext(ctx, `UPDATE reminders SET trigger_at = $1 WHERE id = $2`, time.Now().UTC().Add(-time.Minute), reminder.ID)
	if err != nil {
		t.Fatalf("make reminder due: %v", err)
	}

	group := "notification-e2e-" + uuid.NewString()
	notifierService := newIntegrationNotificationService(t, pg, group)
	taskRepo := taskpostgres.NewRepository(pg.GORM)
	reminderRepo := reminderpostgres.NewRepository(pg.GORM)
	outboxRepo := outboxpostgres.NewRepository(pg.GORM)
	processor, err := reminderscheduler.NewNotificationProcessor(taskRepo, reminderRepo, outboxRepo)
	if err != nil {
		t.Fatalf("NewNotificationProcessor: %v", err)
	}
	runner, err := reminderscheduler.NewRunner(reminderRepo, processor, reminderscheduler.Config{Interval: time.Hour, BatchSize: 10, Workers: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if _, err := runner.ProcessOnce(ctx); err != nil {
		t.Fatalf("scheduler ProcessOnce: %v", err)
	}
	publisher, err := outboxkafka.NewPublisher(cfg.Kafka.Brokers, topic)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer publisher.Close()
	relay, err := outboxservice.NewRelay(outboxRepo, publisher, outboxservice.RelayConfig{Interval: time.Hour, BatchSize: 10}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	if _, err := relay.ProcessOnce(ctx); err != nil {
		t.Fatalf("relay ProcessOnce: %v", err)
	}
	consumer, err := notificationkafka.NewConsumer(cfg.Kafka.Brokers, topic, group, notifierService, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	consumerCtx, stopConsumer := context.WithCancel(ctx)
	defer stopConsumer()
	defer consumer.Close()
	go func() { _ = consumer.Run(consumerCtx) }()

	assigneeID := uuid.MustParse(assignee.ID)
	waitForNotificationCount(ctx, t, pg, assigneeID, 1)

	list := authenticatedRequest(t, router, http.MethodGet, "/api/v1/notifications", nil, assigneeTokens.AccessToken)
	if list.Code != http.StatusOK || len(decodeNotificationHTTPList(t, list).Items) != 1 {
		t.Fatalf("notification list = %d %s, want one item", list.Code, list.Body.String())
	}
	count := authenticatedRequest(t, router, http.MethodGet, "/api/v1/notifications/unread-count", nil, assigneeTokens.AccessToken)
	if decodeUnreadCount(t, count).Count != 1 {
		t.Fatalf("unread count = %s, want 1", count.Body.String())
	}
	notificationID := decodeNotificationHTTPList(t, list).Items[0].ID
	mark := authenticatedRequest(t, router, http.MethodPost, "/api/v1/notifications/"+notificationID+"/read", nil, assigneeTokens.AccessToken)
	if mark.Code != http.StatusNoContent {
		t.Fatalf("mark read status = %d, want 204: %s", mark.Code, mark.Body.String())
	}
	count = authenticatedRequest(t, router, http.MethodGet, "/api/v1/notifications/unread-count", nil, assigneeTokens.AccessToken)
	if decodeUnreadCount(t, count).Count != 0 {
		t.Fatalf("unread count after read = %s, want 0", count.Body.String())
	}
}

func setupNotificationDB(ctx context.Context, t *testing.T, prefix string) (*database.Postgres, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()

	pg := setupTaskHTTPDatabase(ctx, t, prefix)
	users := userpostgres.NewRepository(pg.GORM)
	tasks := taskpostgres.NewRepository(pg.GORM)
	reminders := reminderpostgres.NewRepository(pg.GORM)
	user := createTaskUser(ctx, t, users, prefix+"user@example.com", prefix+"user")
	task := createRepositoryTask(ctx, t, tasks, user.ID, user.ID, prefix+"task", notificationTestTime())
	reminder := mustSchedulerReminder(t, task.ID, notificationTestTime().Add(time.Hour), notificationTestTime(), reminderdomain.StatePending)
	if _, err := reminders.Create(ctx, reminder); err != nil {
		t.Fatalf("create reminder: %v", err)
	}

	return pg, user.ID, task.ID, reminder.ID
}

func newIntegrationNotificationService(t *testing.T, pg *database.Postgres, consumerName string) *notificationservice.NotificationService {
	t.Helper()

	service, err := notificationservice.NewNotificationService(
		notificationpostgres.NewNotificationRepository(pg.GORM),
		notificationpostgres.NewProcessedEventRepository(pg.GORM),
		database.NewTransactionRunner(pg.GORM),
		consumerName,
	)
	if err != nil {
		t.Fatalf("NewNotificationService: %v", err)
	}

	return service
}

func mustNotification(t *testing.T, userID uuid.UUID, taskID uuid.UUID, reminderID uuid.UUID, createdAt time.Time) notificationdomain.Notification {
	t.Helper()

	notification, err := notificationdomain.NewNotification(uuid.New(), userID, &taskID, &reminderID, notificationdomain.TypeTaskReminder, "Task reminder", "A task reminder is due.", createdAt)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}

	return notification
}

func mustNotificationOutboxEvent(t *testing.T, eventID uuid.UUID, userID uuid.UUID, taskID uuid.UUID, reminderID uuid.UUID) outboxdomain.Event {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"reminder_id":       reminderID.String(),
		"task_id":           taskID.String(),
		"recipient_user_id": userID.String(),
		"trigger_at":        notificationTestTime(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	event, err := outboxdomain.NewEvent(eventID, outboxdomain.ReminderAggregateType, reminderID, outboxdomain.NotificationRequestedV1Type, payload, notificationTestTime())
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	return event
}

func countNotifications(ctx context.Context, t *testing.T, pg *database.Postgres, userID uuid.UUID) int {
	t.Helper()

	var count int
	if err := pg.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("count notifications: %v", err)
	}

	return count
}

func waitForNotificationCount(ctx context.Context, t *testing.T, pg *database.Postgres, userID uuid.UUID, want int) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if countNotifications(ctx, t, pg, userID) == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait notification count: %v", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("notification count did not become %d, got %d", want, countNotifications(ctx, t, pg, userID))
}

func processedEventExists(ctx context.Context, t *testing.T, pg *database.Postgres, consumerName string, eventID uuid.UUID) bool {
	t.Helper()

	return countProcessedEvents(ctx, t, pg, consumerName, eventID) > 0
}

func countProcessedEvents(ctx context.Context, t *testing.T, pg *database.Postgres, consumerName string, eventID uuid.UUID) int {
	t.Helper()

	var count int
	if err := pg.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_events WHERE consumer_name = $1 AND event_id = $2`, consumerName, eventID).Scan(&count); err != nil {
		t.Fatalf("count processed events: %v", err)
	}

	return count
}

func decodeNotificationHTTPList(t *testing.T, recorder *httptest.ResponseRecorder) notificationhttp.NotificationListResponse {
	t.Helper()

	var response notificationhttp.NotificationListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode notification list: %v: %s", err, recorder.Body.String())
	}

	return response
}

func decodeUnreadCount(t *testing.T, recorder *httptest.ResponseRecorder) notificationhttp.UnreadCountResponse {
	t.Helper()

	var response notificationhttp.UnreadCountResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode unread count: %v: %s", err, recorder.Body.String())
	}

	return response
}

func createTaskForNotificationHTTP(ctx context.Context, t *testing.T, pg *database.Postgres, userID uuid.UUID) uuid.UUID {
	t.Helper()

	task := createRepositoryTask(ctx, t, taskpostgres.NewRepository(pg.GORM), userID, userID, "notification http task", notificationTestTime())
	return task.ID
}

func createReminderForNotificationHTTP(ctx context.Context, t *testing.T, pg *database.Postgres, taskID uuid.UUID) uuid.UUID {
	t.Helper()

	reminder := mustSchedulerReminder(t, taskID, notificationTestTime().Add(time.Hour), notificationTestTime(), reminderdomain.StatePending)
	if _, err := reminderpostgres.NewRepository(pg.GORM).Create(ctx, reminder); err != nil {
		t.Fatalf("create reminder: %v", err)
	}

	return reminder.ID
}

func notificationTestTime() time.Time {
	return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
}

func uniqueKafkaTopic(base string, suffix string) string {
	return base + "-" + suffix + "-" + uuid.NewString()
}
