package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	outboxout "go_pet_project/internal/outbox/application/port/out"
	"go_pet_project/internal/outbox/domain"

	"github.com/twmb/franz-go/pkg/kgo"
)

var _ outboxout.Publisher = (*Publisher)(nil)

type Publisher struct {
	client *kgo.Client
	topic  string
}

func NewPublisher(brokers []string, topic string) (*Publisher, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers are required")
	}
	if topic == "" {
		return nil, fmt.Errorf("kafka topic is required")
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create kafka client: %w", err)
	}

	return &Publisher{client: client, topic: topic}, nil
}

func (p *Publisher) Publish(ctx context.Context, event domain.Event) error {
	value, err := json.Marshal(newEnvelope(event))
	if err != nil {
		return fmt.Errorf("marshal outbox envelope: %w", err)
	}

	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(event.AggregateID.String()),
		Value: value,
	}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("publish kafka record: %w", err)
	}

	return nil
}

func (p *Publisher) Close() {
	p.client.Close()
}

type envelope struct {
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	Version    int             `json:"version"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func newEnvelope(event domain.Event) envelope {
	return envelope{
		EventID:    event.ID.String(),
		EventType:  event.EventType,
		Version:    domain.EventVersion,
		OccurredAt: event.CreatedAt,
		Payload:    event.Payload,
	}
}
