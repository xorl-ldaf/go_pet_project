package refresh

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	authout "go_pet_project/internal/auth/application/port/out"
)

const DefaultTokenBytes = 32

var _ authout.RefreshTokenGenerator = (*TokenGenerator)(nil)

type TokenGenerator struct {
	tokenBytes int
}

func NewTokenGenerator(tokenBytes int) (*TokenGenerator, error) {
	if tokenBytes == 0 {
		tokenBytes = DefaultTokenBytes
	}
	if tokenBytes < DefaultTokenBytes {
		return nil, fmt.Errorf("refresh token must contain at least %d random bytes", DefaultTokenBytes)
	}

	return &TokenGenerator{tokenBytes: tokenBytes}, nil
}

func (g *TokenGenerator) Generate() (string, error) {
	randomBytes := make([]byte, g.tokenBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func (g *TokenGenerator) Hash(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
