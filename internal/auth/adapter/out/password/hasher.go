package password

import (
	"fmt"

	authout "go_pet_project/internal/auth/application/port/out"

	"golang.org/x/crypto/bcrypt"
)

const DefaultCost = bcrypt.DefaultCost

var _ authout.PasswordHasher = (*Hasher)(nil)

type Hasher struct {
	cost int
}

func NewHasher(cost int) (*Hasher, error) {
	if cost == 0 {
		cost = DefaultCost
	}
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, fmt.Errorf("bcrypt cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}

	return &Hasher{cost: cost}, nil
}

func (h *Hasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return string(hash), nil
}

func (h *Hasher) Compare(password string, encodedHash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password)); err != nil {
		return fmt.Errorf("compare password hash: %w", err)
	}

	return nil
}
