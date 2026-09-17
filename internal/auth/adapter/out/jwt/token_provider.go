package jwt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	authout "go_pet_project/internal/auth/application/port/out"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var _ authout.TokenProvider = (*Provider)(nil)

type Provider struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

type accessClaims struct {
	jwt.RegisteredClaims
}

func NewProvider(secret string, accessTTL time.Duration) (*Provider, error) {
	if len(strings.TrimSpace(secret)) < 32 {
		return nil, fmt.Errorf("JWT secret must be at least 32 characters")
	}
	if accessTTL <= 0 {
		return nil, fmt.Errorf("access token TTL must be positive")
	}

	return &Provider{
		secret:    []byte(secret),
		accessTTL: accessTTL,
		now:       time.Now,
	}, nil
}

func (p *Provider) GenerateAccessToken(userID uuid.UUID) (string, authout.AccessTokenClaims, error) {
	issuedAt := p.now().UTC()
	expiresAt := issuedAt.Add(p.accessTTL)

	claims := accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(p.secret)
	if err != nil {
		return "", authout.AccessTokenClaims{}, fmt.Errorf("sign access token: %w", err)
	}

	return signed, authout.AccessTokenClaims{
		UserID:    userID,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}, nil
}

func (p *Provider) ValidateAccessToken(token string) (authout.AccessTokenClaims, error) {
	claims := &accessClaims{}
	parsed, err := jwt.ParseWithClaims(
		token,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method: %s", token.Header["alg"])
			}

			return p.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(p.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return authout.AccessTokenClaims{}, fmt.Errorf("%w: %w", authout.ErrAccessTokenExpired, err)
		}

		return authout.AccessTokenClaims{}, fmt.Errorf("%w: %w", authout.ErrInvalidAccessToken, err)
	}
	if parsed == nil || !parsed.Valid {
		return authout.AccessTokenClaims{}, authout.ErrInvalidAccessToken
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return authout.AccessTokenClaims{}, fmt.Errorf("%w: invalid subject: %w", authout.ErrInvalidAccessToken, err)
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return authout.AccessTokenClaims{}, fmt.Errorf("%w: missing registered time claims", authout.ErrInvalidAccessToken)
	}

	return authout.AccessTokenClaims{
		UserID:    userID,
		IssuedAt:  claims.IssuedAt.Time,
		ExpiresAt: claims.ExpiresAt.Time,
	}, nil
}
