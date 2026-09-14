package jwt

import (
	"errors"
	"testing"
	"time"

	authout "go_pet_project/internal/auth/application/port/out"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "test-jwt-secret-with-at-least-32-characters"

func TestProviderGenerateAndValidateAccessToken(t *testing.T) {
	now := time.Date(2026, 9, 14, 17, 10, 0, 0, time.UTC)
	provider := newTestProvider(t, testSecret, 15*time.Minute, now)
	userID := uuid.New()

	token, generatedClaims, err := provider.GenerateAccessToken(userID)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	if token == "" {
		t.Fatalf("access token must not be empty")
	}
	if generatedClaims.UserID != userID {
		t.Fatalf("generated UserID = %s, want %s", generatedClaims.UserID, userID)
	}

	validatedClaims, err := provider.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate access token: %v", err)
	}
	if validatedClaims.UserID != userID {
		t.Fatalf("validated UserID = %s, want %s", validatedClaims.UserID, userID)
	}
	if !validatedClaims.ExpiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("ExpiresAt = %s, want %s", validatedClaims.ExpiresAt, now.Add(15*time.Minute))
	}
	if !validatedClaims.IssuedAt.Equal(now) {
		t.Fatalf("IssuedAt = %s, want %s", validatedClaims.IssuedAt, now)
	}
}

func TestProviderRejectsExpiredAccessToken(t *testing.T) {
	now := time.Date(2026, 9, 14, 17, 20, 0, 0, time.UTC)
	provider := newTestProvider(t, testSecret, time.Minute, now)

	token, _, err := provider.GenerateAccessToken(uuid.New())
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	provider.now = func() time.Time {
		return now.Add(2 * time.Minute)
	}

	if _, err := provider.ValidateAccessToken(token); !errors.Is(err, authout.ErrAccessTokenExpired) {
		t.Fatalf("expired token error = %v, want ErrAccessTokenExpired", err)
	}
}

func TestProviderRejectsInvalidSignature(t *testing.T) {
	now := time.Date(2026, 9, 14, 17, 30, 0, 0, time.UTC)
	issuer := newTestProvider(t, "issuer-secret-with-at-least-32-characters", time.Minute, now)
	validator := newTestProvider(t, "validator-secret-with-at-least-32-characters", time.Minute, now)

	token, _, err := issuer.GenerateAccessToken(uuid.New())
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	if _, err := validator.ValidateAccessToken(token); !errors.Is(err, authout.ErrInvalidAccessToken) {
		t.Fatalf("invalid signature error = %v, want ErrInvalidAccessToken", err)
	}
}

func TestProviderRejectsMalformedAccessToken(t *testing.T) {
	provider := newTestProvider(t, testSecret, time.Minute, time.Now().UTC())

	if _, err := provider.ValidateAccessToken("not-a-jwt"); !errors.Is(err, authout.ErrInvalidAccessToken) {
		t.Fatalf("malformed token error = %v, want ErrInvalidAccessToken", err)
	}
}

func TestProviderRejectsUnexpectedAlgorithm(t *testing.T) {
	provider := newTestProvider(t, testSecret, time.Minute, time.Now().UTC())
	claims := accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := provider.ValidateAccessToken(token); !errors.Is(err, authout.ErrInvalidAccessToken) {
		t.Fatalf("unexpected algorithm error = %v, want ErrInvalidAccessToken", err)
	}
}

func newTestProvider(t *testing.T, secret string, ttl time.Duration, now time.Time) *Provider {
	t.Helper()

	provider, err := NewProvider(secret, ttl)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	provider.now = func() time.Time {
		return now
	}

	return provider
}
