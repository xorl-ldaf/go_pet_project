package kafka

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"go_pet_project/internal/notification/application/command"
	"go_pet_project/internal/notification/application/query"
	"go_pet_project/internal/notification/domain"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestHandleRecordCommitsInvalidEventAndSkipsService(t *testing.T) {
	client := &fakeKafkaClient{}
	service := &fakeNotificationService{}
	consumer := newTestConsumer(client, service)

	record := &kgo.Record{Topic: "notifications", Partition: 0, Offset: 10, Value: []byte(`{`)}
	if err := consumer.handleRecord(context.Background(), record); err != nil {
		t.Fatalf("handleRecord: %v", err)
	}
	if service.calls != 0 {
		t.Fatalf("service calls = %d, want 0", service.calls)
	}
	if len(client.committed) != 1 || client.committed[0] != record {
		t.Fatalf("committed records = %d, want invalid record committed", len(client.committed))
	}
}

func TestHandleRecordBusinessErrorDoesNotCommit(t *testing.T) {
	wantErr := errors.New("business error")
	client := &fakeKafkaClient{}
	service := &fakeNotificationService{handleErr: wantErr}
	consumer := newTestConsumer(client, service)

	record := &kgo.Record{Topic: "notifications", Partition: 0, Offset: 10, Value: validNotificationEvent()}
	err := consumer.handleRecord(context.Background(), record)
	if !errors.Is(err, wantErr) {
		t.Fatalf("handleRecord error = %v, want %v", err, wantErr)
	}
	if len(client.committed) != 0 {
		t.Fatalf("committed records = %d, want 0", len(client.committed))
	}
}

func TestRunStopsAfterBusinessErrorWithoutProcessingLaterRecords(t *testing.T) {
	wantErr := errors.New("business error")
	client := &fakeKafkaClient{
		fetches: []kgo.Fetches{{
			{
				Topics: []kgo.FetchTopic{{
					Topic: "notifications",
					Partitions: []kgo.FetchPartition{{
						Partition: 0,
						Records: []*kgo.Record{
							{Topic: "notifications", Partition: 0, Offset: 10, Value: validNotificationEvent()},
							{Topic: "notifications", Partition: 0, Offset: 11, Value: validNotificationEvent()},
						},
					}},
				}},
			},
		}},
	}
	service := &fakeNotificationService{handleErr: wantErr}
	consumer := newTestConsumer(client, service)

	err := consumer.Run(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run error = %v, want %v", err, wantErr)
	}
	if service.calls != 1 {
		t.Fatalf("service calls = %d, want 1", service.calls)
	}
	if len(client.committed) != 0 {
		t.Fatalf("committed records = %d, want 0", len(client.committed))
	}
}

func newTestConsumer(client *fakeKafkaClient, service *fakeNotificationService) *Consumer {
	return &Consumer{
		client:  client,
		service: service,
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func validNotificationEvent() []byte {
	return []byte(`{
		"event_id": "11111111-1111-1111-1111-111111111111",
		"event_type": "notification.requested.v1",
		"version": 1,
		"occurred_at": "2026-09-16T12:00:00Z",
		"payload": {
			"reminder_id": "22222222-2222-2222-2222-222222222222",
			"task_id": "33333333-3333-3333-3333-333333333333",
			"recipient_user_id": "44444444-4444-4444-4444-444444444444",
			"trigger_at": "2026-09-16T12:00:00Z"
		}
	}`)
}

type fakeKafkaClient struct {
	fetches   []kgo.Fetches
	committed []*kgo.Record
	commitErr error
	closed    bool
}

func (c *fakeKafkaClient) PollFetches(ctx context.Context) kgo.Fetches {
	if len(c.fetches) == 0 {
		<-ctx.Done()
		return kgo.Fetches{{Topics: []kgo.FetchTopic{{Partitions: []kgo.FetchPartition{{Err: ctx.Err()}}}}}}
	}
	fetches := c.fetches[0]
	c.fetches = c.fetches[1:]

	return fetches
}

func (c *fakeKafkaClient) CommitRecords(_ context.Context, records ...*kgo.Record) error {
	if c.commitErr != nil {
		return c.commitErr
	}
	c.committed = append(c.committed, records...)

	return nil
}

func (c *fakeKafkaClient) Close() {
	c.closed = true
}

type fakeNotificationService struct {
	calls     int
	handleErr error
}

func (s *fakeNotificationService) HandleNotificationRequested(context.Context, command.HandleNotificationRequestedCommand) error {
	s.calls++

	return s.handleErr
}

func (s *fakeNotificationService) CreateTelegramLinkToken(context.Context, command.CreateTelegramLinkCommand) (command.TelegramLinkTokenResult, error) {
	return command.TelegramLinkTokenResult{}, nil
}

func (s *fakeNotificationService) HandleTelegramStart(context.Context, command.HandleTelegramStartCommand) error {
	return nil
}

func (s *fakeNotificationService) GetNotificationSettings(context.Context, query.GetNotificationSettingsQuery) (query.NotificationSettings, error) {
	return query.NotificationSettings{}, nil
}

func (s *fakeNotificationService) SetTelegramEnabled(context.Context, command.SetTelegramEnabledCommand) error {
	return nil
}

func (s *fakeNotificationService) DeleteTelegramLink(context.Context, command.DeleteTelegramLinkCommand) error {
	return nil
}

func (s *fakeNotificationService) ListNotifications(context.Context, query.ListNotificationsQuery) ([]domain.Notification, error) {
	return nil, nil
}

func (s *fakeNotificationService) CountUnread(context.Context, query.CountUnreadQuery) (int, error) {
	return 0, nil
}

func (s *fakeNotificationService) MarkRead(context.Context, command.MarkReadCommand) error {
	return nil
}

func (s *fakeNotificationService) MarkAllRead(context.Context, command.MarkAllReadCommand) error {
	return nil
}

var _ = time.Time{}
