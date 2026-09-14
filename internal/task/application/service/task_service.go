package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go_pet_project/internal/task/application"
	"go_pet_project/internal/task/application/command"
	taskin "go_pet_project/internal/task/application/port/in"
	taskout "go_pet_project/internal/task/application/port/out"
	taskquery "go_pet_project/internal/task/application/query"
	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

var _ taskin.TaskService = (*TaskService)(nil)

type TaskService struct {
	tasks taskout.TaskRepository
	now   func() time.Time
}

func NewTaskService(tasks taskout.TaskRepository) (*TaskService, error) {
	if tasks == nil {
		return nil, errors.New("task repository is required")
	}

	return &TaskService{
		tasks: tasks,
		now:   time.Now,
	}, nil
}

func (s *TaskService) CreateTask(ctx context.Context, cmd command.CreateTaskCommand) (domain.Task, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return domain.Task{}, err
	}

	assigneeID := cmd.AssigneeID
	if assigneeID == uuid.Nil {
		assigneeID = cmd.ActorID
	}
	if assigneeID != cmd.ActorID {
		return domain.Task{}, application.ErrAssignmentNotSupportedYet
	}

	task, err := domain.NewTask(cmd.ActorID, assigneeID, cmd.Title, cmd.Description, cmd.DeadlineAt, s.now().UTC())
	if err != nil {
		return domain.Task{}, err
	}

	created, err := s.tasks.Create(ctx, task)
	if err != nil {
		return domain.Task{}, fmt.Errorf("create task: %w", err)
	}

	return created, nil
}

func (s *TaskService) GetTask(ctx context.Context, q taskquery.GetTaskQuery) (domain.Task, error) {
	if err := requireActor(q.ActorID); err != nil {
		return domain.Task{}, err
	}

	task, err := s.tasks.FindByID(ctx, q.TaskID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}
	if !canView(q.ActorID, task) {
		return domain.Task{}, application.ErrTaskAccessDenied
	}

	return task, nil
}

func (s *TaskService) ListTasks(ctx context.Context, q taskquery.ListTasksQuery) ([]domain.Task, error) {
	if err := requireActor(q.ActorID); err != nil {
		return nil, err
	}

	filter := taskout.TaskFilter{
		Status:       q.Status,
		Archived:     q.Archived,
		DeadlineFrom: q.DeadlineFrom,
		DeadlineTo:   q.DeadlineTo,
		Overdue:      q.Overdue,
		Now:          q.Now,
		Search:       q.Search,
		Sort:         q.Sort,
	}

	switch q.Scope {
	case taskquery.ListScopeVisible:
		filter.VisibleTo = &q.ActorID
	case taskquery.ListScopeAssignedToMe:
		filter.AssigneeID = &q.ActorID
	case taskquery.ListScopeCreatedByMe:
		filter.CreatorID = &q.ActorID
	default:
		return nil, application.ErrInvalidTaskListScope
	}

	tasks, err := s.tasks.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	return tasks, nil
}

func (s *TaskService) UpdateTask(ctx context.Context, cmd command.UpdateTaskCommand) (domain.Task, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return domain.Task{}, err
	}

	task, err := s.tasks.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}
	if cmd.ActorID != task.CreatorID {
		return domain.Task{}, application.ErrTaskAccessDenied
	}

	now := s.now().UTC()
	changed := false
	if cmd.Title != nil {
		if err := task.UpdateTitle(*cmd.Title, now); err != nil {
			return domain.Task{}, err
		}
		changed = true
	}
	if cmd.Description != nil {
		if err := task.UpdateDescription(*cmd.Description, now); err != nil {
			return domain.Task{}, err
		}
		changed = true
	}
	if cmd.DeadlineAt != nil {
		if err := task.UpdateDeadline(cmd.DeadlineAt.Value, now); err != nil {
			return domain.Task{}, err
		}
		changed = true
	}
	if !changed {
		return task, nil
	}

	updated, err := s.tasks.Update(ctx, task)
	if err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}

	return updated, nil
}

func (s *TaskService) ChangeStatus(ctx context.Context, cmd command.ChangeStatusCommand) (domain.Task, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return domain.Task{}, err
	}

	task, err := s.tasks.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("change task status: %w", err)
	}
	if cmd.ActorID != task.AssigneeID {
		return domain.Task{}, application.ErrTaskAccessDenied
	}
	if err := task.ChangeStatus(cmd.Status, s.now().UTC()); err != nil {
		return domain.Task{}, err
	}

	updated, err := s.tasks.Update(ctx, task)
	if err != nil {
		return domain.Task{}, fmt.Errorf("change task status: %w", err)
	}

	return updated, nil
}

func (s *TaskService) ArchiveTask(ctx context.Context, cmd command.ArchiveTaskCommand) (domain.Task, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return domain.Task{}, err
	}

	task, err := s.tasks.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("archive task: %w", err)
	}
	if cmd.ActorID != task.CreatorID {
		return domain.Task{}, application.ErrTaskAccessDenied
	}
	if err := task.Archive(s.now().UTC()); err != nil {
		return domain.Task{}, err
	}

	updated, err := s.tasks.Update(ctx, task)
	if err != nil {
		return domain.Task{}, fmt.Errorf("archive task: %w", err)
	}

	return updated, nil
}

func (s *TaskService) RestoreTask(ctx context.Context, cmd command.RestoreTaskCommand) (domain.Task, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return domain.Task{}, err
	}

	task, err := s.tasks.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("restore task: %w", err)
	}
	if cmd.ActorID != task.CreatorID {
		return domain.Task{}, application.ErrTaskAccessDenied
	}
	if err := task.Restore(s.now().UTC()); err != nil {
		return domain.Task{}, err
	}

	updated, err := s.tasks.Update(ctx, task)
	if err != nil {
		return domain.Task{}, fmt.Errorf("restore task: %w", err)
	}

	return updated, nil
}

func requireActor(actorID uuid.UUID) error {
	if actorID == uuid.Nil {
		return application.ErrInvalidActor
	}

	return nil
}

func canView(actorID uuid.UUID, task domain.Task) bool {
	return actorID == task.CreatorID || actorID == task.AssigneeID
}
