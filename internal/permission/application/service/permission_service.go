package service

import (
	"context"
	"errors"
	"fmt"

	permissionin "go_pet_project/internal/permission/application/port/in"
	permissionout "go_pet_project/internal/permission/application/port/out"
	"go_pet_project/internal/permission/domain"

	"github.com/google/uuid"
)

var _ permissionin.PermissionService = (*PermissionService)(nil)

type PermissionService struct {
	permissions permissionout.PermissionRepository
}

func NewPermissionService(permissions permissionout.PermissionRepository) (*PermissionService, error) {
	if permissions == nil {
		return nil, errors.New("permission repository is required")
	}

	return &PermissionService{permissions: permissions}, nil
}

func (s *PermissionService) CanAssign(ctx context.Context, assignerID uuid.UUID, assigneeID uuid.UUID) (bool, error) {
	if assignerID == uuid.Nil {
		return false, domain.ErrInvalidAssignerID
	}
	if assigneeID == uuid.Nil {
		return false, domain.ErrInvalidAssigneeID
	}
	if assignerID == assigneeID {
		return true, nil
	}

	allowed, err := s.permissions.Exists(ctx, assignerID, assigneeID)
	if err != nil {
		return false, fmt.Errorf("check assignment permission: %w", err)
	}

	return allowed, nil
}

func (s *PermissionService) ListAssignableUserIDs(ctx context.Context, assignerID uuid.UUID) ([]uuid.UUID, error) {
	if assignerID == uuid.Nil {
		return nil, domain.ErrInvalidAssignerID
	}

	ids, err := s.permissions.ListAssigneeIDs(ctx, assignerID)
	if err != nil {
		return nil, fmt.Errorf("list assignment permissions: %w", err)
	}

	seen := make(map[uuid.UUID]bool, len(ids)+1)
	result := make([]uuid.UUID, 0, len(ids)+1)
	if !seen[assignerID] {
		seen[assignerID] = true
		result = append(result, assignerID)
	}
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}

	return result, nil
}
