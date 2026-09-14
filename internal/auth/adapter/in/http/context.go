package httpadapter

import (
	"context"

	"github.com/google/uuid"
)

type contextKey int

const authenticatedUserIDKey contextKey = iota

func contextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, authenticatedUserIDKey, userID)
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(authenticatedUserIDKey).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return uuid.Nil, false
	}

	return userID, true
}
