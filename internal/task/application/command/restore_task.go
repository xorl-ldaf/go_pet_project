package command

import "github.com/google/uuid"

type RestoreTaskCommand struct {
	ActorID uuid.UUID
	TaskID  uuid.UUID
}
