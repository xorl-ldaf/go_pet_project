package query

import "github.com/google/uuid"

type ListRemindersQuery struct {
	ActorID uuid.UUID
	TaskID  uuid.UUID
}
