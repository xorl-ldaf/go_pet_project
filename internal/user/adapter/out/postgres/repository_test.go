package postgres

import (
	"errors"
	"testing"

	"go_pet_project/internal/user/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapCreateErrorMapsEmailUniqueConstraint(t *testing.T) {
	err := mapCreateError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "users_email_key",
	})

	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("mapCreateError = %v, want ErrEmailAlreadyExists", err)
	}
}

func TestMapCreateErrorMapsUsernameUniqueConstraint(t *testing.T) {
	err := mapCreateError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "users_username_key",
	})

	if !errors.Is(err, domain.ErrUsernameAlreadyExists) {
		t.Fatalf("mapCreateError = %v, want ErrUsernameAlreadyExists", err)
	}
}
