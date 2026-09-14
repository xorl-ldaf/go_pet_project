package postgres

import (
	"fmt"
	"strings"

	taskout "go_pet_project/internal/task/application/port/out"
	"go_pet_project/internal/task/domain"

	"gorm.io/gorm"
)

func applyFilter(query *gorm.DB, filter taskout.TaskFilter) (*gorm.DB, error) {
	if filter.VisibleTo != nil {
		query = query.Where("(creator_id = ? OR assignee_id = ?)", *filter.VisibleTo, *filter.VisibleTo)
	}
	if filter.AssigneeID != nil {
		query = query.Where("assignee_id = ?", *filter.AssigneeID)
	}
	if filter.CreatorID != nil {
		query = query.Where("creator_id = ?", *filter.CreatorID)
	}
	if filter.Status != nil {
		if !filter.Status.IsValid() {
			return nil, fmt.Errorf("%w: invalid status %s", taskout.ErrInvalidTaskFilter, *filter.Status)
		}
		query = query.Where("status = ?", string(*filter.Status))
	}

	switch filter.Archived {
	case taskout.ArchivedActiveOnly:
		query = query.Where("archived_at IS NULL")
	case taskout.ArchivedOnly:
		query = query.Where("archived_at IS NOT NULL")
	case taskout.ArchivedAll:
	default:
		return nil, fmt.Errorf("%w: invalid archived mode %d", taskout.ErrInvalidTaskFilter, filter.Archived)
	}

	if filter.DeadlineFrom != nil && filter.DeadlineTo != nil && filter.DeadlineTo.Before(*filter.DeadlineFrom) {
		return nil, fmt.Errorf("%w: deadline_to must not be before deadline_from", taskout.ErrInvalidTaskFilter)
	}
	if filter.DeadlineFrom != nil {
		query = query.Where("deadline_at >= ?", *filter.DeadlineFrom)
	}
	if filter.DeadlineTo != nil {
		query = query.Where("deadline_at <= ?", *filter.DeadlineTo)
	}

	if filter.Overdue != nil {
		if filter.Now == nil {
			return nil, fmt.Errorf("%w: now is required for overdue filter", taskout.ErrInvalidTaskFilter)
		}

		if *filter.Overdue {
			query = query.Where("deadline_at IS NOT NULL AND deadline_at < ? AND status <> ?", *filter.Now, string(domain.StatusDone))
		} else {
			query = query.Where("(deadline_at IS NULL OR deadline_at >= ? OR status = ?)", *filter.Now, string(domain.StatusDone))
		}
	}

	search := strings.TrimSpace(filter.Search)
	if search != "" {
		pattern := "%" + search + "%"
		query = query.Where("(title ILIKE ? OR description ILIKE ?)", pattern, pattern)
	}

	order, err := orderBy(filter.Sort)
	if err != nil {
		return nil, err
	}

	return query.Order(order), nil
}

func orderBy(sort taskout.TaskSort) (string, error) {
	switch sort {
	case taskout.TaskSortCreatedAtDesc:
		return "created_at DESC, id ASC", nil
	case taskout.TaskSortCreatedAtAsc:
		return "created_at ASC, id ASC", nil
	case taskout.TaskSortDeadlineAtAsc:
		return "deadline_at ASC NULLS LAST, id ASC", nil
	case taskout.TaskSortDeadlineAtDesc:
		return "deadline_at DESC NULLS LAST, id ASC", nil
	default:
		return "", fmt.Errorf("%w: invalid sort %d", taskout.ErrInvalidTaskFilter, sort)
	}
}
