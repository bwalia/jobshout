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

type ProjectService interface {
	Create(ctx context.Context, orgID uuid.UUID, ownerID uuid.UUID, req model.CreateProjectRequest) (*model.Project, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Project, error)
	List(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Project], error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateProjectRequest) (*model.Project, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type projectService struct {
	repo   repository.ProjectRepository
	logger *zap.Logger
}

func NewProjectService(repo repository.ProjectRepository, logger *zap.Logger) ProjectService {
	return &projectService{repo: repo, logger: logger}
}

func (s *projectService) Create(ctx context.Context, orgID uuid.UUID, ownerID uuid.UUID, req model.CreateProjectRequest) (*model.Project, error) {
	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}

	var dueDate *time.Time
	if req.DueDate != nil {
		t, err := time.Parse("2006-01-02", *req.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date format (use YYYY-MM-DD): %w", err)
		}
		dueDate = &t
	}

	project := &model.Project{
		ID:          uuid.New(),
		OrgID:       orgID,
		Name:        req.Name,
		Description: req.Description,
		Status:      "active",
		Priority:    priority,
		OwnerID:     &ownerID,
		DueDate:     dueDate,
	}

	if err := s.repo.Create(ctx, project); err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	return project, nil
}

func (s *projectService) GetByID(ctx context.Context, id uuid.UUID) (*model.Project, error) {
	project, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting project: %w", err)
	}
	if project == nil {
		return nil, ErrProjectNotFound
	}
	return project, nil
}

func (s *projectService) List(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Project], error) {
	return s.repo.ListByOrg(ctx, orgID, params)
}

func (s *projectService) Update(ctx context.Context, id uuid.UUID, req model.UpdateProjectRequest) (*model.Project, error) {
	project, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding project: %w", err)
	}
	if project == nil {
		return nil, ErrProjectNotFound
	}

	if req.Name != nil {
		project.Name = *req.Name
	}
	if req.Description != nil {
		project.Description = req.Description
	}
	if req.Status != nil {
		project.Status = *req.Status
	}
	if req.Priority != nil {
		project.Priority = *req.Priority
	}
	if req.DueDate != nil {
		t, err := time.Parse("2006-01-02", *req.DueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date: %w", err)
		}
		project.DueDate = &t
	}

	if err := s.repo.Update(ctx, project); err != nil {
		return nil, fmt.Errorf("updating project: %w", err)
	}
	return project, nil
}

func (s *projectService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

var ErrProjectNotFound = projectError("project not found")

type projectError string

func (e projectError) Error() string { return string(e) }
