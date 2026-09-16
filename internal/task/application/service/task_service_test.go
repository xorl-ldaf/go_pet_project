package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go_pet_project/internal/task/application"
	"go_pet_project/internal/task/application/command"
	taskout "go_pet_project/internal/task/application/port/out"
	taskquery "go_pet_project/internal/task/application/query"
	"go_pet_project/internal/task/domain"

	"github.com/google/uuid"
)

var errFakeTaskRepository = errors.New("fake task repository error")
var errFakeAssignmentAuthorizer = errors.New("fake assignment authorizer error")

func TestCreateTaskSuccessSelfAssigned(t *testing.T) {
	ctx := context.Background()
	now := testTime()
	actorID := testUUID(1)
	deadline := now.Add(24 * time.Hour)
	tasks := &fakeTaskRepository{}
	service := newTestTaskService(t, tasks, now)

	result, err := service.CreateTask(ctx, command.CreateTaskCommand{
		ActorID:     actorID,
		AssigneeID:  actorID,
		Title:       "Ship Stage 10",
		Description: "application layer",
		DeadlineAt:  &deadline,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if tasks.createCalls != 1 {
		t.Fatalf("Create calls = %d, want 1", tasks.createCalls)
	}
	created := tasks.created[0]
	if created.CreatorID != actorID ||
		created.AssigneeID != actorID ||
		created.Title != "Ship Stage 10" ||
		created.Description != "application layer" ||
		created.Status != domain.InitialStatus ||
		created.DeadlineAt == nil ||
		!created.DeadlineAt.Equal(deadline) ||
		!created.CreatedAt.Equal(now) ||
		!created.UpdatedAt.Equal(now) {
		t.Fatalf("created task has unexpected fields: %#v", created)
	}
	if result.ID != created.ID {
		t.Fatalf("result ID = %s, want created ID %s", result.ID, created.ID)
	}
	if tasks.lastCtx != ctx {
		t.Fatalf("Create did not receive original context")
	}
	if service.assignments.(*fakeAssignmentAuthorizer).calls != 1 {
		t.Fatalf("CanAssign calls = %d, want 1", service.assignments.(*fakeAssignmentAuthorizer).calls)
	}
}

func TestCreateTaskDefaultsAssigneeToActor(t *testing.T) {
	actorID := testUUID(1)
	tasks := &fakeTaskRepository{}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.CreateTask(context.Background(), command.CreateTaskCommand{
		ActorID: actorID,
		Title:   "Self assigned",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if tasks.created[0].CreatorID != actorID || tasks.created[0].AssigneeID != actorID {
		t.Fatalf("created task identities = creator %s assignee %s, want actor %s", tasks.created[0].CreatorID, tasks.created[0].AssigneeID, actorID)
	}
}

func TestCreateTaskAllowedOtherAssignee(t *testing.T) {
	tasks := &fakeTaskRepository{}
	assignments := &fakeAssignmentAuthorizer{allowed: true}
	service := newTestTaskServiceWithAuthorizer(t, tasks, assignments, testTime())
	actorID := testUUID(1)
	assigneeID := testUUID(2)

	_, err := service.CreateTask(context.Background(), command.CreateTaskCommand{
		ActorID:    actorID,
		AssigneeID: assigneeID,
		Title:      "Delegated assignment",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if tasks.createCalls != 1 {
		t.Fatalf("Create calls = %d, want 1", tasks.createCalls)
	}
	if tasks.created[0].CreatorID != actorID || tasks.created[0].AssigneeID != assigneeID {
		t.Fatalf("created task identities mismatch: %#v", tasks.created[0])
	}
	if assignments.lastAssignerID != actorID || assignments.lastAssigneeID != assigneeID {
		t.Fatalf("CanAssign args = %s -> %s, want %s -> %s", assignments.lastAssignerID, assignments.lastAssigneeID, actorID, assigneeID)
	}
}

func TestCreateTaskAssignmentDenied(t *testing.T) {
	tasks := &fakeTaskRepository{}
	service := newTestTaskServiceWithAuthorizer(t, tasks, &fakeAssignmentAuthorizer{allowed: false}, testTime())

	_, err := service.CreateTask(context.Background(), command.CreateTaskCommand{
		ActorID:    testUUID(1),
		AssigneeID: testUUID(2),
		Title:      "Blocked assignment",
	})
	if !errors.Is(err, application.ErrAssignmentDenied) {
		t.Fatalf("CreateTask error = %v, want ErrAssignmentDenied", err)
	}
	if tasks.createCalls != 0 {
		t.Fatalf("Create calls = %d, want 0", tasks.createCalls)
	}
}

func TestCreateTaskAuthorizerFailure(t *testing.T) {
	tasks := &fakeTaskRepository{}
	service := newTestTaskServiceWithAuthorizer(t, tasks, &fakeAssignmentAuthorizer{err: errFakeAssignmentAuthorizer}, testTime())

	_, err := service.CreateTask(context.Background(), command.CreateTaskCommand{
		ActorID:    testUUID(1),
		AssigneeID: testUUID(2),
		Title:      "Authorizer failure",
	})
	if !errors.Is(err, errFakeAssignmentAuthorizer) {
		t.Fatalf("CreateTask error = %v, want authorizer error", err)
	}
	if tasks.createCalls != 0 {
		t.Fatalf("Create calls = %d, want 0", tasks.createCalls)
	}
}

func TestCreateTaskInvalidDomainInput(t *testing.T) {
	tasks := &fakeTaskRepository{}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.CreateTask(context.Background(), command.CreateTaskCommand{
		ActorID: testUUID(1),
		Title:   " ",
	})
	if !errors.Is(err, domain.ErrInvalidTitle) {
		t.Fatalf("CreateTask error = %v, want ErrInvalidTitle", err)
	}
	if tasks.createCalls != 0 {
		t.Fatalf("Create calls = %d, want 0", tasks.createCalls)
	}
}

func TestCreateTaskRepositoryFailure(t *testing.T) {
	tasks := &fakeTaskRepository{createErr: errFakeTaskRepository}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.CreateTask(context.Background(), command.CreateTaskCommand{
		ActorID: testUUID(1),
		Title:   "Repository failure",
	})
	if !errors.Is(err, errFakeTaskRepository) {
		t.Fatalf("CreateTask error = %v, want repository error", err)
	}
}

func TestGetTaskCreator(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime())

	result, err := service.GetTask(context.Background(), taskquery.GetTaskQuery{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
	})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if result.ID != task.ID {
		t.Fatalf("result ID = %s, want %s", result.ID, task.ID)
	}
}

func TestGetTaskAssignee(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.GetTask(context.Background(), taskquery.GetTaskQuery{
		ActorID: task.AssigneeID,
		TaskID:  task.ID,
	})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
}

func TestGetTaskUnrelatedUserDenied(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.GetTask(context.Background(), taskquery.GetTaskQuery{
		ActorID: testUUID(3),
		TaskID:  task.ID,
	})
	if !errors.Is(err, application.ErrTaskAccessDenied) {
		t.Fatalf("GetTask error = %v, want ErrTaskAccessDenied", err)
	}
}

func TestGetTaskArchived(t *testing.T) {
	archivedAt := testTime().Add(time.Hour)
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, &archivedAt)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime())

	result, err := service.GetTask(context.Background(), taskquery.GetTaskQuery{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
	})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if result.ArchivedAt == nil {
		t.Fatalf("archived task should remain accessible")
	}
}

func TestGetTaskRepositoryFailure(t *testing.T) {
	tasks := &fakeTaskRepository{findByIDErr: errFakeTaskRepository}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.GetTask(context.Background(), taskquery.GetTaskQuery{
		ActorID: testUUID(1),
		TaskID:  testUUID(10),
	})
	if !errors.Is(err, errFakeTaskRepository) {
		t.Fatalf("GetTask error = %v, want repository error", err)
	}
}

func TestListTasksVisibilityScope(t *testing.T) {
	actorID := testUUID(1)
	visibleSelf := existingTask(t, actorID, actorID, domain.StatusOpen, nil)
	visibleCreated := existingTask(t, actorID, testUUID(2), domain.StatusOpen, nil)
	visibleAssigned := existingTask(t, testUUID(3), actorID, domain.StatusOpen, nil)
	invisible := existingTask(t, testUUID(3), testUUID(4), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{
		listTasks: []domain.Task{
			visibleSelf,
			visibleCreated,
			visibleAssigned,
			invisible,
		},
	}
	service := newTestTaskService(t, tasks, testTime())

	result, err := service.ListTasks(context.Background(), taskquery.ListTasksQuery{ActorID: actorID})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("visible tasks = %d, want 3", len(result))
	}
	assertTaskIDs(t, result, visibleSelf.ID, visibleCreated.ID, visibleAssigned.ID)
	if tasks.listCalls != 1 || tasks.lastFilter.VisibleTo == nil || *tasks.lastFilter.VisibleTo != actorID {
		t.Fatalf("List filter VisibleTo = %v, want actor %s", tasks.lastFilter.VisibleTo, actorID)
	}
	if tasks.lastFilter.Archived != taskout.ArchivedActiveOnly {
		t.Fatalf("Archived mode = %d, want active only", tasks.lastFilter.Archived)
	}
}

func TestListTasksAssignedToMe(t *testing.T) {
	actorID := testUUID(1)
	tasks := &fakeTaskRepository{}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.ListTasks(context.Background(), taskquery.ListTasksQuery{
		ActorID: actorID,
		Scope:   taskquery.ListScopeAssignedToMe,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if tasks.lastFilter.AssigneeID == nil || *tasks.lastFilter.AssigneeID != actorID {
		t.Fatalf("AssigneeID filter = %v, want actor %s", tasks.lastFilter.AssigneeID, actorID)
	}
	if tasks.lastFilter.VisibleTo != nil || tasks.lastFilter.CreatorID != nil {
		t.Fatalf("unexpected filters: %#v", tasks.lastFilter)
	}
}

func TestListTasksCreatedByMe(t *testing.T) {
	actorID := testUUID(1)
	tasks := &fakeTaskRepository{}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.ListTasks(context.Background(), taskquery.ListTasksQuery{
		ActorID: actorID,
		Scope:   taskquery.ListScopeCreatedByMe,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if tasks.lastFilter.CreatorID == nil || *tasks.lastFilter.CreatorID != actorID {
		t.Fatalf("CreatorID filter = %v, want actor %s", tasks.lastFilter.CreatorID, actorID)
	}
	if tasks.lastFilter.VisibleTo != nil || tasks.lastFilter.AssigneeID != nil {
		t.Fatalf("unexpected filters: %#v", tasks.lastFilter)
	}
}

func TestListTasksPreservesFilters(t *testing.T) {
	actorID := testUUID(1)
	status := domain.StatusInProgress
	deadlineFrom := testTime().Add(24 * time.Hour)
	deadlineTo := testTime().Add(48 * time.Hour)
	overdue := true
	now := testTime()
	tasks := &fakeTaskRepository{}
	service := newTestTaskService(t, tasks, now)

	_, err := service.ListTasks(context.Background(), taskquery.ListTasksQuery{
		ActorID:      actorID,
		Status:       &status,
		Archived:     taskout.ArchivedOnly,
		DeadlineFrom: &deadlineFrom,
		DeadlineTo:   &deadlineTo,
		Overdue:      &overdue,
		Now:          &now,
		Search:       "stage",
		Sort:         taskout.TaskSortDeadlineAtAsc,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	want := taskout.TaskFilter{
		VisibleTo:    &actorID,
		Status:       &status,
		Archived:     taskout.ArchivedOnly,
		DeadlineFrom: &deadlineFrom,
		DeadlineTo:   &deadlineTo,
		Overdue:      &overdue,
		Now:          &now,
		Search:       "stage",
		Sort:         taskout.TaskSortDeadlineAtAsc,
	}
	if !reflect.DeepEqual(tasks.lastFilter, want) {
		t.Fatalf("filter mismatch:\ngot  %#v\nwant %#v", tasks.lastFilter, want)
	}
}

func TestListTasksRepositoryFailure(t *testing.T) {
	tasks := &fakeTaskRepository{listErr: errFakeTaskRepository}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.ListTasks(context.Background(), taskquery.ListTasksQuery{ActorID: testUUID(1)})
	if !errors.Is(err, errFakeTaskRepository) {
		t.Fatalf("ListTasks error = %v, want repository error", err)
	}
}

func TestUpdateTaskCreatorSuccess(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))
	title := "Updated title"

	result, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
		Title:   &title,
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if tasks.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", tasks.updateCalls)
	}
	if result.Title != title || tasks.updated[0].Title != title {
		t.Fatalf("title was not updated through domain method: result=%#v updated=%#v", result, tasks.updated[0])
	}
}

func TestUpdateTaskAssigneeForbidden(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime())
	title := "Forbidden"

	_, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID: task.AssigneeID,
		TaskID:  task.ID,
		Title:   &title,
	})
	if !errors.Is(err, application.ErrTaskAccessDenied) {
		t.Fatalf("UpdateTask error = %v, want ErrTaskAccessDenied", err)
	}
	if tasks.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", tasks.updateCalls)
	}
}

func TestUpdateTaskUnrelatedForbidden(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime())
	title := "Forbidden"

	_, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID: testUUID(3),
		TaskID:  task.ID,
		Title:   &title,
	})
	if !errors.Is(err, application.ErrTaskAccessDenied) {
		t.Fatalf("UpdateTask error = %v, want ErrTaskAccessDenied", err)
	}
}

func TestUpdateTaskPartialUpdate(t *testing.T) {
	deadline := testTime().Add(24 * time.Hour)
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	task.Description = "keep description"
	task.DeadlineAt = &deadline
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))
	title := "Only title"

	result, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
		Title:   &title,
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if result.Description != "keep description" {
		t.Fatalf("Description = %q, want unchanged", result.Description)
	}
	if result.DeadlineAt == nil || !result.DeadlineAt.Equal(deadline) {
		t.Fatalf("DeadlineAt = %v, want unchanged %s", result.DeadlineAt, deadline)
	}
}

func TestUpdateTaskClearsDeadline(t *testing.T) {
	deadline := testTime().Add(24 * time.Hour)
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	task.DeadlineAt = &deadline
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))

	result, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID:    task.CreatorID,
		TaskID:     task.ID,
		DeadlineAt: &command.DeadlineUpdate{Value: nil},
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if result.DeadlineAt != nil {
		t.Fatalf("DeadlineAt = %v, want nil", result.DeadlineAt)
	}
}

func TestUpdateTaskInvalidTitle(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))
	title := ""

	_, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
		Title:   &title,
	})
	if !errors.Is(err, domain.ErrInvalidTitle) {
		t.Fatalf("UpdateTask error = %v, want ErrInvalidTitle", err)
	}
	if tasks.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", tasks.updateCalls)
	}
}

func TestUpdateTaskRepositoryFailure(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task, updateErr: errFakeTaskRepository}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))
	title := "Repository failure"

	_, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
		Title:   &title,
	})
	if !errors.Is(err, errFakeTaskRepository) {
		t.Fatalf("UpdateTask error = %v, want repository error", err)
	}
}

func TestUpdateTaskMixedPatchReassignSuccess(t *testing.T) {
	deadline := testTime().Add(24 * time.Hour)
	task := existingTask(t, testUUID(1), testUUID(1), domain.StatusOpen, nil)
	task.DeadlineAt = &deadline
	tasks := &fakeTaskRepository{findByIDTask: task}
	assignments := &fakeAssignmentAuthorizer{allowed: true}
	service := newTestTaskServiceWithAuthorizer(t, tasks, assignments, testTime().Add(time.Hour))
	title := "Updated title"
	newAssigneeID := testUUID(2)

	result, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID:    task.CreatorID,
		TaskID:     task.ID,
		AssigneeID: &newAssigneeID,
		Title:      &title,
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if tasks.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", tasks.updateCalls)
	}
	if result.CreatorID != task.CreatorID ||
		result.AssigneeID != newAssigneeID ||
		result.Title != title ||
		result.Description != task.Description ||
		result.Status != task.Status ||
		result.DeadlineAt == nil ||
		!result.DeadlineAt.Equal(deadline) {
		t.Fatalf("mixed patch result mismatch: %#v", result)
	}
}

func TestUpdateTaskMixedPatchAssignmentDeniedDoesNotSaveFields(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(1), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskServiceWithAuthorizer(t, tasks, &fakeAssignmentAuthorizer{allowed: false}, testTime().Add(time.Hour))
	title := "Should not save"
	newAssigneeID := testUUID(2)

	_, err := service.UpdateTask(context.Background(), command.UpdateTaskCommand{
		ActorID:    task.CreatorID,
		TaskID:     task.ID,
		AssigneeID: &newAssigneeID,
		Title:      &title,
	})
	if !errors.Is(err, application.ErrAssignmentDenied) {
		t.Fatalf("UpdateTask error = %v, want ErrAssignmentDenied", err)
	}
	if tasks.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", tasks.updateCalls)
	}
}

func TestReassignTaskSuccess(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(1), domain.StatusInProgress, nil)
	task.Title = "Keep title"
	task.Description = "Keep description"
	deadline := testTime().Add(24 * time.Hour)
	task.DeadlineAt = &deadline
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskServiceWithAuthorizer(t, tasks, &fakeAssignmentAuthorizer{allowed: true}, testTime().Add(time.Hour))
	newAssigneeID := testUUID(2)

	result, err := service.ReassignTask(context.Background(), command.ReassignTaskCommand{
		ActorID:       task.CreatorID,
		TaskID:        task.ID,
		NewAssigneeID: newAssigneeID,
	})
	if err != nil {
		t.Fatalf("ReassignTask: %v", err)
	}
	if tasks.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", tasks.updateCalls)
	}
	if result.CreatorID != task.CreatorID ||
		result.AssigneeID != newAssigneeID ||
		result.Title != task.Title ||
		result.Description != task.Description ||
		result.Status != task.Status ||
		result.DeadlineAt == nil ||
		!result.DeadlineAt.Equal(deadline) ||
		result.CompletedAt != task.CompletedAt ||
		result.ArchivedAt != task.ArchivedAt {
		t.Fatalf("reassign result changed unrelated state: %#v", result)
	}
}

func TestReassignTaskAssignmentDenied(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(1), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskServiceWithAuthorizer(t, tasks, &fakeAssignmentAuthorizer{allowed: false}, testTime().Add(time.Hour))

	_, err := service.ReassignTask(context.Background(), command.ReassignTaskCommand{
		ActorID:       task.CreatorID,
		TaskID:        task.ID,
		NewAssigneeID: testUUID(3),
	})
	if !errors.Is(err, application.ErrAssignmentDenied) {
		t.Fatalf("ReassignTask error = %v, want ErrAssignmentDenied", err)
	}
	if tasks.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", tasks.updateCalls)
	}
}

func TestReassignTaskAuthorizerFailure(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(1), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskServiceWithAuthorizer(t, tasks, &fakeAssignmentAuthorizer{err: errFakeAssignmentAuthorizer}, testTime().Add(time.Hour))

	_, err := service.ReassignTask(context.Background(), command.ReassignTaskCommand{
		ActorID:       task.CreatorID,
		TaskID:        task.ID,
		NewAssigneeID: testUUID(3),
	})
	if !errors.Is(err, errFakeAssignmentAuthorizer) {
		t.Fatalf("ReassignTask error = %v, want authorizer error", err)
	}
	if tasks.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", tasks.updateCalls)
	}
}

func TestReassignTaskAssigneeForbidden(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	assignments := &fakeAssignmentAuthorizer{allowed: true}
	service := newTestTaskServiceWithAuthorizer(t, tasks, assignments, testTime().Add(time.Hour))

	_, err := service.ReassignTask(context.Background(), command.ReassignTaskCommand{
		ActorID:       task.AssigneeID,
		TaskID:        task.ID,
		NewAssigneeID: testUUID(3),
	})
	if !errors.Is(err, application.ErrTaskAccessDenied) {
		t.Fatalf("ReassignTask error = %v, want ErrTaskAccessDenied", err)
	}
	if assignments.calls != 0 {
		t.Fatalf("CanAssign calls = %d, want 0", assignments.calls)
	}
}

func TestReassignTaskUnrelatedForbidden(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))

	_, err := service.ReassignTask(context.Background(), command.ReassignTaskCommand{
		ActorID:       testUUID(3),
		TaskID:        task.ID,
		NewAssigneeID: testUUID(4),
	})
	if !errors.Is(err, application.ErrTaskAccessDenied) {
		t.Fatalf("ReassignTask error = %v, want ErrTaskAccessDenied", err)
	}
}

func TestReassignTaskToSelf(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskServiceWithAuthorizer(t, tasks, &fakeAssignmentAuthorizer{allowed: true}, testTime().Add(time.Hour))

	result, err := service.ReassignTask(context.Background(), command.ReassignTaskCommand{
		ActorID:       task.CreatorID,
		TaskID:        task.ID,
		NewAssigneeID: task.CreatorID,
	})
	if err != nil {
		t.Fatalf("ReassignTask: %v", err)
	}
	if result.AssigneeID != task.CreatorID {
		t.Fatalf("AssigneeID = %s, want creator %s", result.AssigneeID, task.CreatorID)
	}
}

func TestReassignTaskCurrentAssigneeNoOp(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	assignments := &fakeAssignmentAuthorizer{allowed: false}
	service := newTestTaskServiceWithAuthorizer(t, tasks, assignments, testTime().Add(time.Hour))

	result, err := service.ReassignTask(context.Background(), command.ReassignTaskCommand{
		ActorID:       task.CreatorID,
		TaskID:        task.ID,
		NewAssigneeID: task.AssigneeID,
	})
	if err != nil {
		t.Fatalf("ReassignTask: %v", err)
	}
	if result.AssigneeID != task.AssigneeID || tasks.updateCalls != 0 || assignments.calls != 0 {
		t.Fatalf("current assignee no-op mismatch: result=%#v updateCalls=%d canAssignCalls=%d", result, tasks.updateCalls, assignments.calls)
	}
}

func TestChangeStatusAssigneeSuccess(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))

	result, err := service.ChangeStatus(context.Background(), command.ChangeStatusCommand{
		ActorID: task.AssigneeID,
		TaskID:  task.ID,
		Status:  domain.StatusInProgress,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.Status != domain.StatusInProgress || tasks.updateCalls != 1 {
		t.Fatalf("status update failed: status=%s updateCalls=%d", result.Status, tasks.updateCalls)
	}
}

func TestChangeStatusCreatorForbidden(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.ChangeStatus(context.Background(), command.ChangeStatusCommand{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
		Status:  domain.StatusInProgress,
	})
	if !errors.Is(err, application.ErrTaskAccessDenied) {
		t.Fatalf("ChangeStatus error = %v, want ErrTaskAccessDenied", err)
	}
	if tasks.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", tasks.updateCalls)
	}
}

func TestChangeStatusInvalidTransition(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusDone, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))

	_, err := service.ChangeStatus(context.Background(), command.ChangeStatusCommand{
		ActorID: task.AssigneeID,
		TaskID:  task.ID,
		Status:  domain.StatusOpen,
	})
	if !errors.Is(err, domain.ErrInvalidStatusTransition) {
		t.Fatalf("ChangeStatus error = %v, want ErrInvalidStatusTransition", err)
	}
	if tasks.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", tasks.updateCalls)
	}
}

func TestChangeStatusCompletion(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	changedAt := testTime().Add(time.Hour)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, changedAt)

	result, err := service.ChangeStatus(context.Background(), command.ChangeStatusCommand{
		ActorID: task.AssigneeID,
		TaskID:  task.ID,
		Status:  domain.StatusDone,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.CompletedAt == nil || !result.CompletedAt.Equal(changedAt) {
		t.Fatalf("CompletedAt = %v, want %s", result.CompletedAt, changedAt)
	}
}

func TestChangeStatusRepositoryFailure(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task, updateErr: errFakeTaskRepository}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))

	_, err := service.ChangeStatus(context.Background(), command.ChangeStatusCommand{
		ActorID: task.AssigneeID,
		TaskID:  task.ID,
		Status:  domain.StatusDone,
	})
	if !errors.Is(err, errFakeTaskRepository) {
		t.Fatalf("ChangeStatus error = %v, want repository error", err)
	}
}

func TestArchiveTaskCreator(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	archivedAt := testTime().Add(time.Hour)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, archivedAt)

	result, err := service.ArchiveTask(context.Background(), command.ArchiveTaskCommand{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
	})
	if err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	if result.ArchivedAt == nil || !result.ArchivedAt.Equal(archivedAt) {
		t.Fatalf("ArchivedAt = %v, want %s", result.ArchivedAt, archivedAt)
	}
	if tasks.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", tasks.updateCalls)
	}
}

func TestArchiveTaskAssigneeForbidden(t *testing.T) {
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, nil)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.ArchiveTask(context.Background(), command.ArchiveTaskCommand{
		ActorID: task.AssigneeID,
		TaskID:  task.ID,
	})
	if !errors.Is(err, application.ErrTaskAccessDenied) {
		t.Fatalf("ArchiveTask error = %v, want ErrTaskAccessDenied", err)
	}
	if tasks.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", tasks.updateCalls)
	}
}

func TestRestoreTaskCreator(t *testing.T) {
	archivedAt := testTime().Add(30 * time.Minute)
	task := existingTask(t, testUUID(1), testUUID(2), domain.StatusOpen, &archivedAt)
	tasks := &fakeTaskRepository{findByIDTask: task}
	service := newTestTaskService(t, tasks, testTime().Add(time.Hour))

	result, err := service.RestoreTask(context.Background(), command.RestoreTaskCommand{
		ActorID: task.CreatorID,
		TaskID:  task.ID,
	})
	if err != nil {
		t.Fatalf("RestoreTask: %v", err)
	}
	if result.ArchivedAt != nil {
		t.Fatalf("ArchivedAt = %v, want nil", result.ArchivedAt)
	}
	if tasks.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", tasks.updateCalls)
	}
}

func TestArchiveTaskFindFailure(t *testing.T) {
	tasks := &fakeTaskRepository{findByIDErr: errFakeTaskRepository}
	service := newTestTaskService(t, tasks, testTime())

	_, err := service.ArchiveTask(context.Background(), command.ArchiveTaskCommand{
		ActorID: testUUID(1),
		TaskID:  testUUID(10),
	})
	if !errors.Is(err, errFakeTaskRepository) {
		t.Fatalf("ArchiveTask error = %v, want repository error", err)
	}
}

func newTestTaskService(t *testing.T, tasks *fakeTaskRepository, now time.Time) *TaskService {
	t.Helper()

	return newTestTaskServiceWithAuthorizer(t, tasks, &fakeAssignmentAuthorizer{allowed: true}, now)
}

func newTestTaskServiceWithAuthorizer(t *testing.T, tasks *fakeTaskRepository, assignments *fakeAssignmentAuthorizer, now time.Time) *TaskService {
	t.Helper()

	service, err := NewTaskService(tasks, assignments, &fakeReminderManager{}, &fakeTransactionRunner{})
	if err != nil {
		t.Fatalf("new task service: %v", err)
	}
	service.now = func() time.Time {
		return now
	}

	return service
}

type fakeTransactionRunner struct {
	calls int
}

func (r *fakeTransactionRunner) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	r.calls++
	return fn(ctx)
}

type fakeReminderManager struct {
	recalculateErr error
	cancelErr      error

	recalculateCalls int
	cancelCalls      int

	lastRecalculateTaskID   uuid.UUID
	lastRecalculateDeadline *time.Time
	lastCancelTaskID        uuid.UUID
}

func (m *fakeReminderManager) RecalculatePendingBeforeDeadline(_ context.Context, taskID uuid.UUID, deadlineAt *time.Time) error {
	m.recalculateCalls++
	m.lastRecalculateTaskID = taskID
	if deadlineAt != nil {
		copied := *deadlineAt
		m.lastRecalculateDeadline = &copied
	} else {
		m.lastRecalculateDeadline = nil
	}
	if m.recalculateErr != nil {
		return m.recalculateErr
	}

	return nil
}

func (m *fakeReminderManager) CancelPending(_ context.Context, taskID uuid.UUID) error {
	m.cancelCalls++
	m.lastCancelTaskID = taskID
	if m.cancelErr != nil {
		return m.cancelErr
	}

	return nil
}

type fakeAssignmentAuthorizer struct {
	allowed bool
	err     error

	calls          int
	lastAssignerID uuid.UUID
	lastAssigneeID uuid.UUID
}

func (a *fakeAssignmentAuthorizer) CanAssign(_ context.Context, assignerID uuid.UUID, assigneeID uuid.UUID) (bool, error) {
	a.calls++
	a.lastAssignerID = assignerID
	a.lastAssigneeID = assigneeID
	if a.err != nil {
		return false, a.err
	}

	return a.allowed, nil
}

type fakeTaskRepository struct {
	createErr error
	updateErr error
	listErr   error

	findByIDTask domain.Task
	findByIDErr  error

	listTasks []domain.Task

	created []domain.Task
	updated []domain.Task

	createCalls int
	findCalls   int
	updateCalls int
	listCalls   int

	lastFindByID uuid.UUID
	lastFilter   taskout.TaskFilter
	lastCtx      context.Context
}

func (r *fakeTaskRepository) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	r.createCalls++
	r.lastCtx = ctx
	r.created = append(r.created, task)
	if r.createErr != nil {
		return domain.Task{}, r.createErr
	}

	return task, nil
}

func (r *fakeTaskRepository) FindByID(ctx context.Context, id uuid.UUID) (domain.Task, error) {
	r.findCalls++
	r.lastCtx = ctx
	r.lastFindByID = id
	if r.findByIDErr != nil {
		return domain.Task{}, r.findByIDErr
	}

	return r.findByIDTask, nil
}

func (r *fakeTaskRepository) Update(ctx context.Context, task domain.Task) (domain.Task, error) {
	r.updateCalls++
	r.lastCtx = ctx
	r.updated = append(r.updated, task)
	if r.updateErr != nil {
		return domain.Task{}, r.updateErr
	}

	return task, nil
}

func (r *fakeTaskRepository) List(ctx context.Context, filter taskout.TaskFilter) ([]domain.Task, error) {
	r.listCalls++
	r.lastCtx = ctx
	r.lastFilter = filter
	if r.listErr != nil {
		return nil, r.listErr
	}

	result := make([]domain.Task, 0, len(r.listTasks))
	for _, task := range r.listTasks {
		if filter.VisibleTo != nil && task.CreatorID != *filter.VisibleTo && task.AssigneeID != *filter.VisibleTo {
			continue
		}
		if filter.AssigneeID != nil && task.AssigneeID != *filter.AssigneeID {
			continue
		}
		if filter.CreatorID != nil && task.CreatorID != *filter.CreatorID {
			continue
		}
		if filter.Status != nil && task.Status != *filter.Status {
			continue
		}
		switch filter.Archived {
		case taskout.ArchivedActiveOnly:
			if task.ArchivedAt != nil {
				continue
			}
		case taskout.ArchivedOnly:
			if task.ArchivedAt == nil {
				continue
			}
		}
		result = append(result, task)
	}

	return result, nil
}

func existingTask(t *testing.T, creatorID uuid.UUID, assigneeID uuid.UUID, status domain.Status, archivedAt *time.Time) domain.Task {
	t.Helper()

	now := testTime()
	var completedAt *time.Time
	if status.IsCompleted() {
		completedAt = &now
	}

	task, err := domain.RestoreTask(
		uuid.New(),
		nil,
		creatorID,
		assigneeID,
		"Existing task",
		"description",
		status,
		nil,
		now,
		now,
		completedAt,
		archivedAt,
	)
	if err != nil {
		t.Fatalf("RestoreTask: %v", err)
	}

	return task
}

func testTime() time.Time {
	return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
}

func testUUID(n int) uuid.UUID {
	return uuid.MustParse("10000000-0000-4000-8000-" + leftPadInt(n, 12))
}

func assertTaskIDs(t *testing.T, tasks []domain.Task, want ...uuid.UUID) {
	t.Helper()

	if len(tasks) != len(want) {
		t.Fatalf("task count = %d, want %d", len(tasks), len(want))
	}
	got := make(map[uuid.UUID]bool, len(tasks))
	for _, task := range tasks {
		got[task.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Fatalf("expected task %s in result", id)
		}
	}
}

func leftPadInt(n int, width int) string {
	value := ""
	for n > 0 {
		value = string(rune('0'+n%10)) + value
		n /= 10
	}
	for len(value) < width {
		value = "0" + value
	}

	return value
}
