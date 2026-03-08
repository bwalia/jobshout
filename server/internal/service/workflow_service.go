package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	wfengine "github.com/jobshout/server/internal/workflow"
)

// WorkflowService handles CRUD and execution of multi-agent workflows.
type WorkflowService interface {
	Create(ctx context.Context, orgID uuid.UUID, createdBy uuid.UUID, req model.CreateWorkflowRequest) (*model.Workflow, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Workflow, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Workflow], error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateWorkflowRequest) (*model.Workflow, error)
	Delete(ctx context.Context, id uuid.UUID) error

	Execute(ctx context.Context, wfID uuid.UUID, orgID uuid.UUID, triggeredBy uuid.UUID, req model.ExecuteWorkflowRequest) (*model.WorkflowRun, error)
	GetRunByID(ctx context.Context, id uuid.UUID) (*model.WorkflowRun, error)
	ListRuns(ctx context.Context, workflowID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.WorkflowRun], error)
}

type workflowService struct {
	wfRepo       repository.WorkflowRepository
	agentRepo    repository.AgentRepository
	execRepo     repository.ExecutionRepository
	toolPermRepo repository.AgentToolRepository
	dagEngine    *wfengine.Engine
	logger       *zap.Logger
}

// NewWorkflowService creates a WorkflowService wired to the given DAG engine.
func NewWorkflowService(
	wfRepo repository.WorkflowRepository,
	agentRepo repository.AgentRepository,
	execRepo repository.ExecutionRepository,
	toolPermRepo repository.AgentToolRepository,
	dagEngine *wfengine.Engine,
	logger *zap.Logger,
) WorkflowService {
	return &workflowService{
		wfRepo:       wfRepo,
		agentRepo:    agentRepo,
		execRepo:     execRepo,
		toolPermRepo: toolPermRepo,
		dagEngine:    dagEngine,
		logger:       logger,
	}
}

func (s *workflowService) Create(ctx context.Context, orgID uuid.UUID, createdBy uuid.UUID, req model.CreateWorkflowRequest) (*model.Workflow, error) {
	wf := &model.Workflow{
		ID:          uuid.New(),
		OrgID:       orgID,
		Name:        req.Name,
		Description: req.Description,
		Status:      "draft",
		CreatedBy:   &createdBy,
	}

	for i, sr := range req.Steps {
		agentID, err := uuid.Parse(sr.AgentID)
		if err != nil {
			return nil, fmt.Errorf("workflow_svc: invalid agent_id in step %d: %w", i, err)
		}
		wf.Steps = append(wf.Steps, model.WorkflowStep{
			ID:            uuid.New(),
			WorkflowID:    wf.ID,
			Name:          sr.Name,
			AgentID:       agentID,
			InputTemplate: sr.InputTemplate,
			Position:      sr.Position,
			DependsOn:     sr.DependsOn,
		})
	}

	if err := s.wfRepo.Create(ctx, wf); err != nil {
		return nil, fmt.Errorf("workflow_svc: create: %w", err)
	}
	return wf, nil
}

func (s *workflowService) GetByID(ctx context.Context, id uuid.UUID) (*model.Workflow, error) {
	wf, err := s.wfRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflow_svc: get: %w", err)
	}
	return wf, nil
}

func (s *workflowService) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Workflow], error) {
	return s.wfRepo.ListByOrg(ctx, orgID, params)
}

func (s *workflowService) Update(ctx context.Context, id uuid.UUID, req model.UpdateWorkflowRequest) (*model.Workflow, error) {
	wf, err := s.wfRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflow_svc: find for update: %w", err)
	}

	if req.Name != nil {
		wf.Name = *req.Name
	}
	if req.Description != nil {
		wf.Description = req.Description
	}
	if req.Status != nil {
		wf.Status = *req.Status
	}

	if err := s.wfRepo.Update(ctx, wf); err != nil {
		return nil, fmt.Errorf("workflow_svc: update: %w", err)
	}
	return wf, nil
}

func (s *workflowService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.wfRepo.Delete(ctx, id)
}

func (s *workflowService) Execute(ctx context.Context, wfID uuid.UUID, orgID uuid.UUID, triggeredBy uuid.UUID, req model.ExecuteWorkflowRequest) (*model.WorkflowRun, error) {
	wf, err := s.wfRepo.GetByID(ctx, wfID)
	if err != nil {
		return nil, fmt.Errorf("workflow_svc: load workflow: %w", err)
	}

	input := req.Input
	if input == nil {
		input = map[string]any{}
	}

	run := &model.WorkflowRun{
		ID:          uuid.New(),
		WorkflowID:  wfID,
		OrgID:       orgID,
		Status:      model.WorkflowRunStatusPending,
		Input:       input,
		Outputs:     map[string]string{},
		TriggeredBy: &triggeredBy,
	}

	if err := s.wfRepo.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("workflow_svc: create run record: %w", err)
	}

	// Mark as running.
	now := time.Now()
	run.Status = model.WorkflowRunStatusRunning
	run.StartedAt = &now
	_ = s.wfRepo.UpdateRun(ctx, run)

	// Execute asynchronously so the HTTP handler can return immediately with
	// the run record, and callers can poll GET /runs/{id} for the result.
	go s.runAsync(context.Background(), wf, run, input)

	return run, nil
}

func (s *workflowService) runAsync(ctx context.Context, wf *model.Workflow, run *model.WorkflowRun, input map[string]any) {
	log := s.logger.With(
		zap.String("workflow_id", wf.ID.String()),
		zap.String("run_id", run.ID.String()),
	)
	log.Info("workflow run started")

	outputs, err := s.dagEngine.Execute(ctx, wf, run, input)

	completedAt := time.Now()
	run.CompletedAt = &completedAt
	run.Outputs = outputs

	if err != nil {
		log.Error("workflow run failed", zap.Error(err))
		run.Status = model.WorkflowRunStatusFailed
		errMsg := err.Error()
		run.ErrorMessage = &errMsg
	} else {
		log.Info("workflow run completed", zap.Int("steps", len(outputs)))
		run.Status = model.WorkflowRunStatusCompleted
	}

	if updateErr := s.wfRepo.UpdateRun(ctx, run); updateErr != nil {
		log.Error("failed to update run record", zap.Error(updateErr))
	}
}

func (s *workflowService) GetRunByID(ctx context.Context, id uuid.UUID) (*model.WorkflowRun, error) {
	return s.wfRepo.GetRunByID(ctx, id)
}

func (s *workflowService) ListRuns(ctx context.Context, workflowID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.WorkflowRun], error) {
	return s.wfRepo.ListRuns(ctx, workflowID, params)
}

// dagPersister adapts ExecutionRepository to the wfengine.ExecutionPersister interface.
type dagPersister struct {
	execRepo repository.ExecutionRepository
}

func NewDagPersister(execRepo repository.ExecutionRepository) wfengine.ExecutionPersister {
	return &dagPersister{execRepo: execRepo}
}

func (p *dagPersister) RecordStarted(ctx context.Context, execID uuid.UUID, agentID uuid.UUID, orgID uuid.UUID, runID uuid.UUID, stepID uuid.UUID, prompt string) error {
	exec := &model.AgentExecution{
		ID:            execID,
		AgentID:       agentID,
		OrgID:         orgID,
		WorkflowRunID: &runID,
		StepID:        &stepID,
		InputPrompt:   prompt,
		Status:        model.ExecutionStatusPending,
	}
	if err := p.execRepo.Create(ctx, exec); err != nil {
		return err
	}
	return p.execRepo.MarkStarted(ctx, execID)
}

func (p *dagPersister) RecordCompleted(ctx context.Context, execID uuid.UUID, result executor.Result) error {
	return p.execRepo.PersistResult(ctx, execID, result)
}
