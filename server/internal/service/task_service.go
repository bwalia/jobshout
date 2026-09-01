package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type TaskService interface {
	Create(ctx context.Context, createdBy uuid.UUID, req model.CreateTaskRequest) (*model.Task, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Task, error)
	List(ctx context.Context, projectID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error)
	ListComments(ctx context.Context, taskID uuid.UUID) ([]model.TaskComment, error)
	AddComment(ctx context.Context, taskID uuid.UUID, authorID uuid.UUID, body string) (*model.TaskComment, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateTaskRequest) (*model.Task, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Transition(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error
	Reorder(ctx context.Context, id uuid.UUID, status string, position int, changedBy *uuid.UUID) error
	History(ctx context.Context, id uuid.UUID) (*model.TaskHistory, error)
	FindByLaunchRunID(ctx context.Context, runID uuid.UUID) (*model.Task, error)
}

type taskService struct {
	repo   repository.TaskRepository
	logger *zap.Logger
}

func NewTaskService(repo repository.TaskRepository, logger *zap.Logger) TaskService {
	return &taskService{repo: repo, logger: logger}
}

func (s *taskService) Create(ctx context.Context, createdBy uuid.UUID, req model.CreateTaskRequest) (*model.Task, error) {
	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("invalid project_id: %w", err)
	}

	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}

	status := req.Status
	if status == "" {
		status = "backlog"
	}

	var parentID *uuid.UUID
	if req.ParentID != nil {
		p, err := uuid.Parse(*req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent_id: %w", err)
		}
		parentID = &p
	}

	var assignedAgentID *uuid.UUID
	if req.AssignedAgentID != nil {
		a, err := uuid.Parse(*req.AssignedAgentID)
		if err != nil {
			return nil, fmt.Errorf("invalid assigned_agent_id: %w", err)
		}
		assignedAgentID = &a
	}

	var assignedUserID *uuid.UUID
	if req.AssignedUserID != nil {
		u, err := uuid.Parse(*req.AssignedUserID)
		if err != nil {
			return nil, fmt.Errorf("invalid assigned_user_id: %w", err)
		}
		assignedUserID = &u
	}

	var dueDate *time.Time
	if req.DueDate != nil {
		t, err := time.Parse("2006-01-02", *req.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date: %w", err)
		}
		dueDate = &t
	}

	task := &model.Task{
		ID:              uuid.New(),
		ProjectID:       projectID,
		ParentID:        parentID,
		Title:           req.Title,
		Description:     req.Description,
		Status:          status,
		Priority:        priority,
		AssignedAgentID: assignedAgentID,
		AssignedUserID:  assignedUserID,
		StoryPoints:     req.StoryPoints,
		DueDate:         dueDate,
		CreatedBy:       &createdBy,
		Metadata:        req.Metadata,
	}

	if err := s.repo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("creating task: %w", err)
	}
	return task, nil
}

func (s *taskService) GetByID(ctx context.Context, id uuid.UUID) (*model.Task, error) {
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting task: %w", err)
	}
	if task == nil {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

func (s *taskService) List(ctx context.Context, projectID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	return s.repo.ListByProject(ctx, projectID, params)
}

func (s *taskService) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	return s.repo.ListByOrg(ctx, orgID, params)
}

func (s *taskService) ListComments(ctx context.Context, taskID uuid.UUID) ([]model.TaskComment, error) {
	return s.repo.ListComments(ctx, taskID)
}

func (s *taskService) AddComment(ctx context.Context, taskID uuid.UUID, authorID uuid.UUID, body string) (*model.TaskComment, error) {
	comment := &model.TaskComment{
		ID:       uuid.New(),
		TaskID:   taskID,
		AuthorID: &authorID,
		Body:     body,
	}
	if err := s.repo.AddComment(ctx, comment); err != nil {
		return nil, fmt.Errorf("adding comment: %w", err)
	}
	return comment, nil
}

func (s *taskService) Update(ctx context.Context, id uuid.UUID, req model.UpdateTaskRequest) (*model.Task, error) {
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding task: %w", err)
	}
	if task == nil {
		return nil, ErrTaskNotFound
	}

	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = req.Description
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.StoryPoints != nil {
		task.StoryPoints = req.StoryPoints
	}
	if req.AssignedAgentID.Set {
		id, err := parseOptionalUUID(req.AssignedAgentID)
		if err != nil {
			return nil, fmt.Errorf("invalid assigned_agent_id: %w", err)
		}
		task.AssignedAgentID = id
	}
	if req.AssignedUserID.Set {
		id, err := parseOptionalUUID(req.AssignedUserID)
		if err != nil {
			return nil, fmt.Errorf("invalid assigned_user_id: %w", err)
		}
		task.AssignedUserID = id
	}
	if req.DueDate.Set {
		if req.DueDate.Value == nil || strings.TrimSpace(*req.DueDate.Value) == "" {
			task.DueDate = nil
		} else {
			t, err := time.Parse("2006-01-02", strings.TrimSpace(*req.DueDate.Value))
			if err != nil {
				return nil, fmt.Errorf("invalid due_date: %w", err)
			}
			task.DueDate = &t
		}
	}
	if req.Metadata != nil {
		task.Metadata = req.Metadata
	}

	nextStatus := ""
	if req.Status != nil {
		nextStatus = strings.TrimSpace(*req.Status)
		if nextStatus != "" {
			switch nextStatus {
			case "backlog", "todo", "in_progress", "review", "done":
			default:
				return nil, ErrInvalidTaskStatus
			}
		}
	}

	if err := s.repo.Update(ctx, task); err != nil {
		return nil, fmt.Errorf("updating task: %w", err)
	}
	if nextStatus != "" && nextStatus != task.Status {
		if err := s.repo.TransitionStatus(ctx, id, nextStatus, nil); err != nil {
			return nil, fmt.Errorf("updating status: %w", err)
		}
		return s.GetByID(ctx, id)
	}
	return task, nil
}

func (s *taskService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *taskService) Transition(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error {
	return s.repo.TransitionStatus(ctx, id, status, changedBy)
}

func (s *taskService) Reorder(ctx context.Context, id uuid.UUID, status string, position int, changedBy *uuid.UUID) error {
	return s.repo.Reorder(ctx, id, status, position, changedBy)
}

func (s *taskService) History(ctx context.Context, id uuid.UUID) (*model.TaskHistory, error) {
	if _, err := s.GetByID(ctx, id); err != nil {
		return nil, err
	}
	hist, err := s.repo.ListHistory(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting task history: %w", err)
	}
	if hist == nil {
		return nil, ErrTaskNotFound
	}
	return hist, nil
}

func (s *taskService) FindByLaunchRunID(ctx context.Context, runID uuid.UUID) (*model.Task, error) {
	return s.repo.FindByLaunchRunID(ctx, runID)
}

func parseOptionalUUID(field model.OptionalString) (*uuid.UUID, error) {
	if field.Value == nil || strings.TrimSpace(*field.Value) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(strings.TrimSpace(*field.Value))
	if err != nil {
		return nil, err
	}
	return &id, nil
}

var ErrTaskNotFound = taskError("task not found")
var ErrInvalidTaskStatus = taskError("invalid status")

type taskError string

func (e taskError) Error() string { return string(e) }
