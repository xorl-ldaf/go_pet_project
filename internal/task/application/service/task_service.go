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
	tasks        taskout.TaskRepository
	assignments  taskout.AssignmentAuthorizer
	reminders    taskout.ReminderManager
	transactions taskout.TransactionRunner
	now          func() time.Time
}

func NewTaskService(
	tasks taskout.TaskRepository,
	assignments taskout.AssignmentAuthorizer,
	reminders taskout.ReminderManager,
	transactions taskout.TransactionRunner,
) (*TaskService, error) {
	if tasks == nil {
		return nil, errors.New("task repository is required")
	}
	if assignments == nil {
		return nil, errors.New("assignment authorizer is required")
	}
	if reminders == nil {
		return nil, errors.New("reminder manager is required")
	}
	if transactions == nil {
		return nil, errors.New("transaction runner is required")
	}

	return &TaskService{
		tasks:        tasks,
		assignments:  assignments,
		reminders:    reminders,
		transactions: transactions,
		now:          time.Now,
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
	allowed, err := s.assignments.CanAssign(ctx, cmd.ActorID, assigneeID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("authorize task assignment: %w", err)
	}
	if !allowed {
		return domain.Task{}, application.ErrAssignmentDenied
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
	var updated domain.Task
	err := s.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		updated, err = s.updateTask(txCtx, cmd)
		return err
	})
	if err != nil {
		return domain.Task{}, err
	}

	return updated, nil
}

func (s *TaskService) updateTask(ctx context.Context, cmd command.UpdateTaskCommand) (domain.Task, error) {
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
	deadlineChanged := false
	if cmd.AssigneeID != nil && *cmd.AssigneeID != task.AssigneeID {
		if err := s.reassignLoadedTask(ctx, cmd.ActorID, &task, *cmd.AssigneeID, now); err != nil {
			return domain.Task{}, err
		}
		changed = true
	}
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
		deadlineChanged = !sameTimePtr(task.DeadlineAt, cmd.DeadlineAt.Value)
		if deadlineChanged {
			if err := task.UpdateDeadline(cmd.DeadlineAt.Value, now); err != nil {
				return domain.Task{}, err
			}
			changed = true
		}
	}
	if !changed {
		return task, nil
	}

	if deadlineChanged {
		if err := s.reminders.RecalculatePendingBeforeDeadline(ctx, task.ID, task.DeadlineAt); err != nil {
			return domain.Task{}, fmt.Errorf("sync reminders after deadline update: %w", err)
		}
	}

	updated, err := s.tasks.Update(ctx, task)
	if err != nil {
		return domain.Task{}, fmt.Errorf("update task: %w", err)
	}

	return updated, nil
}

func (s *TaskService) ReassignTask(ctx context.Context, cmd command.ReassignTaskCommand) (domain.Task, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return domain.Task{}, err
	}

	task, err := s.tasks.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("reassign task: %w", err)
	}
	if cmd.ActorID != task.CreatorID {
		return domain.Task{}, application.ErrTaskAccessDenied
	}
	if cmd.NewAssigneeID == task.AssigneeID {
		return task, nil
	}
	if err := s.reassignLoadedTask(ctx, cmd.ActorID, &task, cmd.NewAssigneeID, s.now().UTC()); err != nil {
		return domain.Task{}, err
	}

	updated, err := s.tasks.Update(ctx, task)
	if err != nil {
		return domain.Task{}, fmt.Errorf("reassign task: %w", err)
	}

	return updated, nil
}

func (s *TaskService) ChangeStatus(ctx context.Context, cmd command.ChangeStatusCommand) (domain.Task, error) {
	var updated domain.Task
	err := s.transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		updated, err = s.changeStatus(txCtx, cmd)
		return err
	})
	if err != nil {
		return domain.Task{}, err
	}

	return updated, nil
}

func (s *TaskService) changeStatus(ctx context.Context, cmd command.ChangeStatusCommand) (domain.Task, error) {
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

	if task.Status.IsCompleted() {
		if err := s.reminders.CancelPending(ctx, task.ID); err != nil {
			return domain.Task{}, fmt.Errorf("cancel reminders after task completion: %w", err)
		}
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

func sameTimePtr(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.Equal(*right)
}

func (s *TaskService) reassignLoadedTask(ctx context.Context, actorID uuid.UUID, task *domain.Task, assigneeID uuid.UUID, now time.Time) error {
	if assigneeID == uuid.Nil {
		return domain.ErrInvalidAssigneeID
	}
	allowed, err := s.assignments.CanAssign(ctx, actorID, assigneeID)
	if err != nil {
		return fmt.Errorf("authorize task assignment: %w", err)
	}
	if !allowed {
		return application.ErrAssignmentDenied
	}
	if err := task.Reassign(assigneeID, now); err != nil {
		return err
	}

	return nil
}
