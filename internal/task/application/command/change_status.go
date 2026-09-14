package command

import (
	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

type ChangeStatusCommand struct {
	ActorID uuid.UUID
	TaskID  uuid.UUID
	Status  domain.Status
}
