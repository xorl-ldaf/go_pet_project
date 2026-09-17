package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("SCHEDULER_INTERVAL", "not-a-duration")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SCHEDULER_INTERVAL must be a valid duration") {
		t.Fatalf("Load error = %v, want invalid scheduler duration", err)
	}
}

func TestValidateRejectsInvalidRuntimeConfig(t *testing.T) {
	cfg := validTestConfig()
	cfg.Scheduler.BatchSize = 0

	err := cfg.validate()
	if err == nil || err.Error() != "SCHEDULER_BATCH_SIZE must be positive" {
		t.Fatalf("validate error = %v, want scheduler batch size error", err)
	}
}

func TestValidateRequiresJWTSecret(t *testing.T) {
	cfg := validTestConfig()
	cfg.Auth.JWTSecret = ""

	err := cfg.validate()
	if err == nil || err.Error() != "JWT_SECRET must be at least 32 characters" {
		t.Fatalf("validate error = %v, want JWT secret error", err)
	}
}

func TestLoadSchedulerDoesNotRequireJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")

	cfg, err := LoadScheduler()
	if err != nil {
		t.Fatalf("LoadScheduler: %v", err)
	}
	if cfg.Auth.JWTSecret != "" {
		t.Fatalf("JWTSecret = %q, want empty value allowed for scheduler", cfg.Auth.JWTSecret)
	}
}

func TestLoadNotifierDoesNotRequireJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")

	cfg, err := LoadNotifier()
	if err != nil {
		t.Fatalf("LoadNotifier: %v", err)
	}
	if cfg.Auth.JWTSecret != "" {
		t.Fatalf("JWTSecret = %q, want empty value allowed for notifier", cfg.Auth.JWTSecret)
	}
}

func TestLoadAPIRequiresJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "short")

	_, err := LoadAPI()
	if err == nil || err.Error() != "JWT_SECRET must be at least 32 characters" {
		t.Fatalf("LoadAPI error = %v, want JWT secret error", err)
	}
}

func TestLoadSchedulerRejectsNonPositiveSchedulerInterval(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("SCHEDULER_INTERVAL", "0s")

	_, err := LoadScheduler()
	if err == nil || err.Error() != "SCHEDULER_INTERVAL must be positive" {
		t.Fatalf("LoadScheduler error = %v, want scheduler interval error", err)
	}
}

func TestLoadSchedulerRejectsNonPositiveRecurrenceBatchSize(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("RECURRENCE_BATCH_SIZE", "0")

	_, err := LoadScheduler()
	if err == nil || err.Error() != "RECURRENCE_BATCH_SIZE must be positive" {
		t.Fatalf("LoadScheduler error = %v, want recurrence batch size error", err)
	}
}

func TestLoadNotifierRejectsNonPositiveDeliveryBatchSize(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("TELEGRAM_DELIVERY_BATCH_SIZE", "0")

	_, err := LoadNotifier()
	if err == nil || err.Error() != "TELEGRAM_DELIVERY_BATCH_SIZE must be positive" {
		t.Fatalf("LoadNotifier error = %v, want telegram delivery batch size error", err)
	}
}

func validTestConfig() *Config {
	return &Config{
		HTTP: HTTPConfig{Port: 8080},
		DB: DBConfig{
			Host:     "localhost",
			Port:     5432,
			Name:     "todo",
			User:     "todo",
			Password: "todo",
			SSLMode:  "disable",
		},
		Auth: AuthConfig{
			JWTSecret:       "test-jwt-secret-with-at-least-32-bytes",
			AccessTokenTTL:  time.Minute,
			RefreshTokenTTL: time.Hour,
		},
		Scheduler: schedulerConfig(),
		Recurrence: RecurrenceConfig{
			Interval:  time.Second,
			BatchSize: 10,
		},
		Kafka: KafkaConfig{
			Brokers:           []string{"localhost:9092"},
			NotificationTopic: "notification.requested.v1",
			NotificationGroup: "todo-notifier-v1",
		},
		Outbox: OutboxConfig{
			Interval:  time.Second,
			BatchSize: 10,
		},
		Telegram: TelegramConfig{
			RetryDelays:       []time.Duration{time.Second},
			DeliveryInterval:  time.Second,
			DeliveryBatchSize: 10,
		},
		Metrics: MetricsConfig{Port: 0},
	}
}

func schedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		Interval:  time.Second,
		BatchSize: 10,
		Workers:   1,
	}
}
