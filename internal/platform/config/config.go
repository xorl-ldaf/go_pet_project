package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	defaultHTTPPort = 8080
	defaultDBHost   = "localhost"
	defaultDBPort   = 5432
	defaultDBName   = "todo_dev"
	defaultDBUser   = "todo"
	defaultDBPass   = "todo"
	defaultSSLMode  = "disable"

	defaultJWTSecret       = ""
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 30 * 24 * time.Hour

	defaultSchedulerInterval  = 5 * time.Second
	defaultSchedulerBatchSize = 100
	defaultSchedulerWorkers   = 1

	defaultRecurrenceInterval  = 5 * time.Second
	defaultRecurrenceBatchSize = 100

	defaultKafkaBrokers           = "localhost:9092"
	defaultKafkaNotificationTopic = "notification.requested.v1"
	defaultKafkaNotificationGroup = "todo-notifier-v1"

	defaultOutboxInterval  = 2 * time.Second
	defaultOutboxBatchSize = 25

	defaultTelegramRetryDelays       = "10s,1m,5m"
	defaultTelegramDeliveryInterval  = 5 * time.Second
	defaultTelegramDeliveryBatchSize = 25

	defaultMetricsPort = 0
)

type Config struct {
	HTTP       HTTPConfig
	DB         DBConfig
	Auth       AuthConfig
	Scheduler  SchedulerConfig
	Recurrence RecurrenceConfig
	Kafka      KafkaConfig
	Outbox     OutboxConfig
	Telegram   TelegramConfig
	Metrics    MetricsConfig
}

type HTTPConfig struct {
	Port int
}

type DBConfig struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
}

type AuthConfig struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type SchedulerConfig struct {
	Interval  time.Duration
	BatchSize int
	Workers   int
}

type RecurrenceConfig struct {
	Interval  time.Duration
	BatchSize int
}

type KafkaConfig struct {
	Brokers           []string
	NotificationTopic string
	NotificationGroup string
}

type OutboxConfig struct {
	Interval  time.Duration
	BatchSize int
}

type TelegramConfig struct {
	BotToken          string
	BotUsername       string
	RetryDelays       []time.Duration
	DeliveryInterval  time.Duration
	DeliveryBatchSize int
}

type MetricsConfig struct {
	Port int
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	httpPort, err := getInt("HTTP_PORT", defaultHTTPPort)
	if err != nil {
		return nil, err
	}
	if err := validatePort("HTTP_PORT", httpPort); err != nil {
		return nil, err
	}

	dbPort, err := getInt("DB_PORT", defaultDBPort)
	if err != nil {
		return nil, err
	}
	if err := validatePort("DB_PORT", dbPort); err != nil {
		return nil, err
	}

	accessTokenTTL, err := getDuration("ACCESS_TOKEN_TTL", defaultAccessTokenTTL)
	if err != nil {
		return nil, err
	}

	refreshTokenTTL, err := getDuration("REFRESH_TOKEN_TTL", defaultRefreshTokenTTL)
	if err != nil {
		return nil, err
	}

	schedulerInterval, err := getDuration("SCHEDULER_INTERVAL", defaultSchedulerInterval)
	if err != nil {
		return nil, err
	}

	schedulerBatchSize, err := getInt("SCHEDULER_BATCH_SIZE", defaultSchedulerBatchSize)
	if err != nil {
		return nil, err
	}

	schedulerWorkers, err := getInt("SCHEDULER_WORKERS", defaultSchedulerWorkers)
	if err != nil {
		return nil, err
	}

	recurrenceInterval, err := getDuration("RECURRENCE_INTERVAL", defaultRecurrenceInterval)
	if err != nil {
		return nil, err
	}

	recurrenceBatchSize, err := getInt("RECURRENCE_BATCH_SIZE", defaultRecurrenceBatchSize)
	if err != nil {
		return nil, err
	}

	outboxInterval, err := getDuration("OUTBOX_INTERVAL", defaultOutboxInterval)
	if err != nil {
		return nil, err
	}

	outboxBatchSize, err := getInt("OUTBOX_BATCH_SIZE", defaultOutboxBatchSize)
	if err != nil {
		return nil, err
	}

	telegramRetryDelays, err := getDurationCSV("TELEGRAM_RETRY_DELAYS", defaultTelegramRetryDelays)
	if err != nil {
		return nil, err
	}

	telegramDeliveryInterval, err := getDuration("TELEGRAM_DELIVERY_INTERVAL", defaultTelegramDeliveryInterval)
	if err != nil {
		return nil, err
	}

	telegramDeliveryBatchSize, err := getInt("TELEGRAM_DELIVERY_BATCH_SIZE", defaultTelegramDeliveryBatchSize)
	if err != nil {
		return nil, err
	}

	metricsPort, err := getInt("METRICS_PORT", defaultMetricsPort)
	if err != nil {
		return nil, err
	}
	if metricsPort != 0 {
		if err := validatePort("METRICS_PORT", metricsPort); err != nil {
			return nil, err
		}
	}

	cfg := &Config{
		HTTP: HTTPConfig{
			Port: httpPort,
		},
		DB: DBConfig{
			Host:     getString("DB_HOST", defaultDBHost),
			Port:     dbPort,
			Name:     getString("DB_NAME", defaultDBName),
			User:     getString("DB_USER", defaultDBUser),
			Password: getString("DB_PASSWORD", defaultDBPass),
			SSLMode:  getString("DB_SSLMODE", defaultSSLMode),
		},
		Auth: AuthConfig{
			JWTSecret:       getString("JWT_SECRET", defaultJWTSecret),
			AccessTokenTTL:  accessTokenTTL,
			RefreshTokenTTL: refreshTokenTTL,
		},
		Scheduler: SchedulerConfig{
			Interval:  schedulerInterval,
			BatchSize: schedulerBatchSize,
			Workers:   schedulerWorkers,
		},
		Recurrence: RecurrenceConfig{
			Interval:  recurrenceInterval,
			BatchSize: recurrenceBatchSize,
		},
		Kafka: KafkaConfig{
			Brokers:           getCSV("KAFKA_BROKERS", defaultKafkaBrokers),
			NotificationTopic: getString("KAFKA_NOTIFICATION_TOPIC", defaultKafkaNotificationTopic),
			NotificationGroup: getString("KAFKA_NOTIFICATION_CONSUMER_GROUP", defaultKafkaNotificationGroup),
		},
		Outbox: OutboxConfig{
			Interval:  outboxInterval,
			BatchSize: outboxBatchSize,
		},
		Telegram: TelegramConfig{
			BotToken:          getString("TELEGRAM_BOT_TOKEN", ""),
			BotUsername:       strings.TrimPrefix(getString("TELEGRAM_BOT_USERNAME", ""), "@"),
			RetryDelays:       telegramRetryDelays,
			DeliveryInterval:  telegramDeliveryInterval,
			DeliveryBatchSize: telegramDeliveryBatchSize,
		},
		Metrics: MetricsConfig{
			Port: metricsPort,
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DB.Host == "" {
		return errors.New("DB_HOST must not be empty")
	}
	if c.DB.Name == "" {
		return errors.New("DB_NAME must not be empty")
	}
	if c.DB.User == "" {
		return errors.New("DB_USER must not be empty")
	}
	if c.DB.Password == "" {
		return errors.New("DB_PASSWORD must not be empty")
	}
	if c.DB.SSLMode == "" {
		return errors.New("DB_SSLMODE must not be empty")
	}
	if len(c.Auth.JWTSecret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters")
	}
	if c.Auth.AccessTokenTTL <= 0 {
		return errors.New("ACCESS_TOKEN_TTL must be positive")
	}
	if c.Auth.RefreshTokenTTL <= 0 {
		return errors.New("REFRESH_TOKEN_TTL must be positive")
	}
	if c.Scheduler.Interval <= 0 {
		return errors.New("SCHEDULER_INTERVAL must be positive")
	}
	if c.Scheduler.BatchSize <= 0 {
		return errors.New("SCHEDULER_BATCH_SIZE must be positive")
	}
	if c.Scheduler.Workers <= 0 {
		return errors.New("SCHEDULER_WORKERS must be positive")
	}
	if c.Recurrence.Interval <= 0 {
		return errors.New("RECURRENCE_INTERVAL must be positive")
	}
	if c.Recurrence.BatchSize <= 0 {
		return errors.New("RECURRENCE_BATCH_SIZE must be positive")
	}
	if len(c.Kafka.Brokers) == 0 {
		return errors.New("KAFKA_BROKERS must not be empty")
	}
	for _, broker := range c.Kafka.Brokers {
		if broker == "" {
			return errors.New("KAFKA_BROKERS must not contain empty brokers")
		}
	}
	if c.Kafka.NotificationTopic == "" {
		return errors.New("KAFKA_NOTIFICATION_TOPIC must not be empty")
	}
	if c.Kafka.NotificationGroup == "" {
		return errors.New("KAFKA_NOTIFICATION_CONSUMER_GROUP must not be empty")
	}
	if c.Outbox.Interval <= 0 {
		return errors.New("OUTBOX_INTERVAL must be positive")
	}
	if c.Outbox.BatchSize <= 0 {
		return errors.New("OUTBOX_BATCH_SIZE must be positive")
	}
	if len(c.Telegram.RetryDelays) == 0 {
		return errors.New("TELEGRAM_RETRY_DELAYS must not be empty")
	}
	for _, delay := range c.Telegram.RetryDelays {
		if delay <= 0 {
			return errors.New("TELEGRAM_RETRY_DELAYS must contain only positive durations")
		}
	}
	if c.Telegram.DeliveryInterval <= 0 {
		return errors.New("TELEGRAM_DELIVERY_INTERVAL must be positive")
	}
	if c.Telegram.DeliveryBatchSize <= 0 {
		return errors.New("TELEGRAM_DELIVERY_BATCH_SIZE must be positive")
	}
	if c.Metrics.Port < 0 {
		return errors.New("METRICS_PORT must be between 1 and 65535 or 0 to disable")
	}
	if c.Metrics.Port > 0 {
		if err := validatePort("METRICS_PORT", c.Metrics.Port); err != nil {
			return err
		}
	}

	return nil
}

func (c HTTPConfig) Address() string {
	return net.JoinHostPort("", strconv.Itoa(c.Port))
}

func (c DBConfig) URL() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:   c.Name,
	}

	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	u.RawQuery = q.Encode()

	return u.String()
}

func (c DBConfig) RedactedURL() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.User(c.User),
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:   c.Name,
	}

	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	u.RawQuery = q.Encode()

	return u.String()
}

func getString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func getCSV(key, fallback string) []string {
	raw := getString(key, fallback)
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}

	return values
}

func getDurationCSV(key, fallback string) ([]time.Duration, error) {
	values := getCSV(key, fallback)
	durations := make([]time.Duration, 0, len(values))
	for _, value := range values {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return nil, fmt.Errorf("%s must contain valid durations: %w", key, err)
		}
		durations = append(durations, duration)
	}

	return durations, nil
}

func getInt(key string, fallback int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}

	return value, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}

	return value, nil
}

func validatePort(key string, value int) error {
	if value < 1 || value > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", key)
	}

	return nil
}
