package query

import "github.com/google/uuid"

type GetTaskQuery struct {
	ActorID uuid.UUID
	TaskID  uuid.UUID
}
