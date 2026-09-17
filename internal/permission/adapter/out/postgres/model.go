package postgres

import (
	"time"

	"github.com/google/uuid"
)

type assignmentPermissionModel struct {
	AssignerID uuid.UUID `gorm:"column:assigner_id;type:uuid;primaryKey"`
	AssigneeID uuid.UUID `gorm:"column:assignee_id;type:uuid;primaryKey"`
	CreatedAt  time.Time `gorm:"column:created_at;type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
}

func (assignmentPermissionModel) TableName() string {
	return "assignment_permissions"
}
