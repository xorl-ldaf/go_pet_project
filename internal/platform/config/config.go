package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
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

	defaultJWTSecret       = "dev-only-jwt-secret-change-me-32-bytes-minimum"
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 30 * 24 * time.Hour
)

type Config struct {
	HTTP HTTPConfig
	DB   DBConfig
	Auth AuthConfig
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
