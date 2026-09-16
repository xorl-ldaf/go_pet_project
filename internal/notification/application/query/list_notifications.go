package query

import "github.com/google/uuid"

type ListNotificationsQuery struct {
	ActorID uuid.UUID
}

type CountUnreadQuery struct {
	ActorID uuid.UUID
}
