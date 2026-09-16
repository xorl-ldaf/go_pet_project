package in

import (
	"context"

	"go_pet_project/internal/task/application/command"
	"go_pet_project/internal/task/application/query"
	"go_pet_project/internal/task/domain"
)

type TaskService interface {
	CreateTask(ctx context.Context, cmd command.CreateTaskCommand) (domain.Task, error)
	GetTask(ctx context.Context, q query.GetTaskQuery) (domain.Task, error)
	ListTasks(ctx context.Context, q query.ListTasksQuery) ([]domain.Task, error)
	UpdateTask(ctx context.Context, cmd command.UpdateTaskCommand) (domain.Task, error)
	ReassignTask(ctx context.Context, cmd command.ReassignTaskCommand) (domain.Task, error)
	ChangeStatus(ctx context.Context, cmd command.ChangeStatusCommand) (domain.Task, error)
	ArchiveTask(ctx context.Context, cmd command.ArchiveTaskCommand) (domain.Task, error)
	RestoreTask(ctx context.Context, cmd command.RestoreTaskCommand) (domain.Task, error)
}
