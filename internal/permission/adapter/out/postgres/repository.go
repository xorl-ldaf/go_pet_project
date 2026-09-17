package postgres

import (
	"context"
	"fmt"

	permissionout "go_pet_project/internal/permission/application/port/out"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var _ permissionout.PermissionRepository = (*Repository)(nil)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Exists(ctx context.Context, assignerID uuid.UUID, assigneeID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.db.WithContext(ctx).
		Raw(`
			SELECT EXISTS (
				SELECT 1
				FROM assignment_permissions
				WHERE assigner_id = ? AND assignee_id = ?
			)
		`, assignerID, assigneeID).
		Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("check assignment permission exists: %w", err)
	}

	return exists, nil
}

func (r *Repository) ListAssigneeIDs(ctx context.Context, assignerID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).
		Model(&assignmentPermissionModel{}).
		Where("assigner_id = ?", assignerID).
		Order("assignee_id ASC").
		Pluck("assignee_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list assignment permission assignees: %w", err)
	}

	return ids, nil
}
