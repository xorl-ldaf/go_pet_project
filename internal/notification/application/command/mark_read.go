package command

import "github.com/google/uuid"

type MarkReadCommand struct {
	ActorID        uuid.UUID
	NotificationID uuid.UUID
}

type MarkAllReadCommand struct {
	ActorID uuid.UUID
}
