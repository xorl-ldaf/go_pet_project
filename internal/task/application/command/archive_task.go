package command

import "github.com/google/uuid"

type ArchiveTaskCommand struct {
	ActorID uuid.UUID
	TaskID  uuid.UUID
}
