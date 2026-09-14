package refresh

import (
	"encoding/base64"
	"testing"
)

func TestTokenGenerator(t *testing.T) {
	generator, err := NewTokenGenerator(DefaultTokenBytes)
	if err != nil {
		t.Fatalf("new token generator: %v", err)
	}

	first, err := generator.Generate()
	if err != nil {
		t.Fatalf("generate first token: %v", err)
	}
	second, err := generator.Generate()
	if err != nil {
		t.Fatalf("generate second token: %v", err)
	}

	if first == "" || second == "" {
		t.Fatalf("generated refresh token must not be empty")
	}
	if first == second {
		t.Fatalf("separate refresh token generations must differ")
	}
	if _, err := base64.RawURLEncoding.DecodeString(first); err != nil {
		t.Fatalf("generated token must use raw URL-safe base64 encoding: %v", err)
	}

	firstHash := generator.Hash(first)
	if firstHash == "" {
		t.Fatalf("hash must not be empty")
	}
	if firstHash != generator.Hash(first) {
		t.Fatalf("hash must be deterministic")
	}
	if firstHash == generator.Hash(second) {
		t.Fatalf("different tokens must have different hashes")
	}
	if firstHash == first {
		t.Fatalf("hash must not equal plain token")
	}
}
