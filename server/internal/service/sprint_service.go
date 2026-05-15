package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// SprintService handles sprint CRUD and assembles the sprint detail view.
type SprintService interface {
	Create(ctx context.Context, orgID uuid.UUID, createdBy *uuid.UUID, req model.CreateSprintRequest) (*model.Sprint, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Sprint, error)
	GetDetail(ctx context.Context, id uuid.UUID) (*model.SprintDetail, error)
	List(ctx context.Context, orgID uuid.UUID) ([]model.Sprint, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateSprintRequest) (*model.Sprint, error)
	Delete(ctx context.Context, id uuid.UUID) error

	AddJob(ctx context.Context, sprintID uuid.UUID, req model.AddSprintJobRequest) error
	RemoveJob(ctx context.Context, sprintID, jobID uuid.UUID) error
	AddAgent(ctx context.Context, sprintID uuid.UUID, req model.AddSprintAgentRequest) error
	RemoveAgent(ctx context.Context, sprintID, agentID uuid.UUID, roleLabel string) error
}

type sprintService struct {
	repo repository.SprintRepository
}

func NewSprintService(repo repository.SprintRepository) SprintService {
	return &sprintService{repo: repo}
}

func (s *sprintService) Create(ctx context.Context, orgID uuid.UUID, createdBy *uuid.UUID, req model.CreateSprintRequest) (*model.Sprint, error) {
	sp := &model.Sprint{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      req.Name,
		Goal:      req.Goal,
		Status:    model.SprintStatusPlanning,
		StartAt:   req.StartAt,
		EndAt:     req.EndAt,
		CreatedBy: createdBy,
	}
	if err := s.repo.Create(ctx, sp); err != nil {
		return nil, fmt.Errorf("sprint_svc: create: %w", err)
	}
	return sp, nil
}

func (s *sprintService) Get(ctx context.Context, id uuid.UUID) (*model.Sprint, error) {
	return s.repo.Get(ctx, id)
}

func (s *sprintService) GetDetail(ctx context.Context, id uuid.UUID) (*model.SprintDetail, error) {
	sp, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	jobs, err := s.repo.ListJobs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("sprint_svc: list jobs: %w", err)
	}
	agents, err := s.repo.ListAgents(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("sprint_svc: list agents: %w", err)
	}

	stats := model.SprintStats{TotalJobs: len(jobs)}
	for _, j := range jobs {
		switch j.Status {
		case model.MultiAgentStatusCompleted:
			stats.CompletedJobs++
		case model.MultiAgentStatusFailed:
			stats.FailedJobs++
		case model.MultiAgentStatusPending,
			model.MultiAgentStatusPlanning,
			model.MultiAgentStatusExecuting,
			model.MultiAgentStatusReviewing:
			stats.InFlightJobs++
		}
	}

	return &model.SprintDetail{
		Sprint: *sp,
		Jobs:   jobs,
		Agents: agents,
		Stats:  stats,
	}, nil
}

func (s *sprintService) List(ctx context.Context, orgID uuid.UUID) ([]model.Sprint, error) {
	return s.repo.ListByOrg(ctx, orgID)
}

func (s *sprintService) Update(ctx context.Context, id uuid.UUID, req model.UpdateSprintRequest) (*model.Sprint, error) {
	return s.repo.Update(ctx, id, req)
}

func (s *sprintService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *sprintService) AddJob(ctx context.Context, sprintID uuid.UUID, req model.AddSprintJobRequest) error {
	return s.repo.AddJob(ctx, sprintID, req.JobID, req.Position)
}

func (s *sprintService) RemoveJob(ctx context.Context, sprintID, jobID uuid.UUID) error {
	return s.repo.RemoveJob(ctx, sprintID, jobID)
}

func (s *sprintService) AddAgent(ctx context.Context, sprintID uuid.UUID, req model.AddSprintAgentRequest) error {
	return s.repo.AddAgent(ctx, sprintID, req.AgentID, req.RoleLabel)
}

func (s *sprintService) RemoveAgent(ctx context.Context, sprintID, agentID uuid.UUID, roleLabel string) error {
	if roleLabel == "" {
		roleLabel = "any"
	}
	return s.repo.RemoveAgent(ctx, sprintID, agentID, roleLabel)
}
