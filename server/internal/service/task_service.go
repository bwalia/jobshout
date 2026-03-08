package service

import (
	"context"
	"fmt"
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
	Update(ctx context.Context, id uuid.UUID, req model.UpdateTaskRequest) (*model.Task, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Transition(ctx context.Context, id uuid.UUID, status string) error
	Reorder(ctx context.Context, id uuid.UUID, status string, position int) error
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
		Status:          "backlog",
		Priority:        priority,
		AssignedAgentID: assignedAgentID,
		AssignedUserID:  assignedUserID,
		StoryPoints:     req.StoryPoints,
		DueDate:         dueDate,
		CreatedBy:       &createdBy,
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
	if req.AssignedAgentID != nil {
		a, _ := uuid.Parse(*req.AssignedAgentID)
		task.AssignedAgentID = &a
	}
	if req.AssignedUserID != nil {
		u, _ := uuid.Parse(*req.AssignedUserID)
		task.AssignedUserID = &u
	}
	if req.DueDate != nil {
		t, _ := time.Parse("2006-01-02", *req.DueDate)
		task.DueDate = &t
	}

	if err := s.repo.Update(ctx, task); err != nil {
		return nil, fmt.Errorf("updating task: %w", err)
	}
	return task, nil
}

func (s *taskService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *taskService) Transition(ctx context.Context, id uuid.UUID, status string) error {
	return s.repo.TransitionStatus(ctx, id, status)
}

func (s *taskService) Reorder(ctx context.Context, id uuid.UUID, status string, position int) error {
	return s.repo.Reorder(ctx, id, status, position)
}

var ErrTaskNotFound = taskError("task not found")

type taskError string

func (e taskError) Error() string { return string(e) }
