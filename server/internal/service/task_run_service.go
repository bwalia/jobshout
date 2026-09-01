package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ErrTaskRunNotFound is returned when a run does not exist, or exists but
// belongs to a different organization — the two cases are deliberately
// indistinguishable across the org boundary, matching PentestService.
var ErrTaskRunNotFound = errors.New("task run not found")

// TaskRunService launches and reads on-demand agent runs of a board task.
type TaskRunService interface {
	// CreateRun validates the task and agent against orgID, writes a queued run,
	// and launches it asynchronously. It returns the queued run immediately; the
	// caller polls GetRun for progress. requestedBy records who launched it and
	// may be nil.
	CreateRun(ctx context.Context, taskID uuid.UUID, req model.CreateTaskRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.TaskRun, error)
	// GetRun returns the run only if it belongs to orgID; otherwise ErrTaskRunNotFound.
	GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.TaskRun, error)
	// ListRuns lists the runs of a task, scoped to orgID.
	ListRuns(ctx context.Context, taskID, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.TaskRun], error)
}

type taskRunService struct {
	runRepo     repository.TaskRunRepository
	taskRepo    repository.TaskRepository
	projectRepo repository.ProjectRepository
	agentRepo   repository.AgentRepository
	execSvc     ExecutionService
	logger      *zap.Logger
}

func NewTaskRunService(
	runRepo repository.TaskRunRepository,
	taskRepo repository.TaskRepository,
	projectRepo repository.ProjectRepository,
	agentRepo repository.AgentRepository,
	execSvc ExecutionService,
	logger *zap.Logger,
) TaskRunService {
	return &taskRunService{
		runRepo:     runRepo,
		taskRepo:    taskRepo,
		projectRepo: projectRepo,
		agentRepo:   agentRepo,
		execSvc:     execSvc,
		logger:      logger,
	}
}

func (s *taskRunService) CreateRun(ctx context.Context, taskID uuid.UUID, req model.CreateTaskRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.TaskRun, error) {
	// Resolve the task and confirm it belongs to the caller's org via its
	// project. Tasks carry no org_id of their own — the boundary is the project.
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return nil, ErrTaskRunNotFound
	}
	project, err := s.projectRepo.FindByID(ctx, task.ProjectID)
	if err != nil || project == nil || project.OrgID != orgID {
		return nil, ErrTaskRunNotFound
	}

	// Resolve which agent runs the task: an explicit override wins, else the
	// task's assigned agent. A task with neither cannot be run.
	agentID, err := resolveRunAgent(req.AgentID, task.AssignedAgentID)
	if err != nil {
		return nil, err
	}
	agent, err := s.agentRepo.FindByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found")
	}
	if agent.OrgID != orgID {
		// Never run one org's task with another org's agent.
		return nil, fmt.Errorf("agent does not belong to organization")
	}

	prompt := buildRunPrompt(task, req)
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("nothing to run: the task has no description and no prompt was provided")
	}

	run := &model.TaskRun{
		ID:            uuid.New(),
		TaskID:        taskID,
		AgentID:       agentID,
		OrgID:         orgID,
		Status:        model.TaskRunStatusQueued,
		Prompt:        prompt,
		Engine:        req.Engine,
		ModelProvider: req.ModelProvider,
		ModelName:     req.ModelName,
		SkillSlugs:    normalizeSlugs(req.SkillSlugs),
		Inputs:        req.Inputs,
		Debug:         req.Debug,
		RequestedBy:   requestedBy,
	}
	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create task run: %w", err)
	}

	s.syncBoardStatus(ctx, taskID, run.Status, requestedBy)

	// Launch asynchronously and return the queued run at once: the HTTP request
	// must not block for the whole agent run (it can take minutes and would trip
	// the client's timeout). A detached context keeps the run alive past the
	// request. The Task Manager polls GetRun for progress.
	go s.execute(context.Background(), run, agentID)

	s.logger.Info("task run queued",
		zap.String("run_id", run.ID.String()),
		zap.String("task_id", taskID.String()),
		zap.String("agent_id", agentID.String()),
	)
	return run, nil
}

// execute drives one queued run to a terminal state. It runs in its own
// goroutine with a detached context; every failure path writes a terminal run
// row so a run is never left stuck as "running".
func (s *taskRunService) execute(ctx context.Context, run *model.TaskRun, agentID uuid.UUID) {
	now := time.Now()
	run.Status = model.TaskRunStatusRunning
	run.StartedAt = &now
	if err := s.runRepo.Update(ctx, run); err != nil {
		s.logger.Warn("failed to mark task run running", zap.String("run_id", run.ID.String()), zap.Error(err))
	}

	exec, err := s.execSvc.Execute(ctx, run.OrgID, agentID, model.ExecuteAgentRequest{
		Prompt:         run.Prompt,
		EngineOverride: run.Engine,
		ModelProvider:  run.ModelProvider,
		ModelName:      run.ModelName,
		SkillSlugs:     run.SkillSlugs,
	})

	completedAt := time.Now()
	run.CompletedAt = &completedAt

	if err != nil {
		run.Status = model.TaskRunStatusFailed
		msg := err.Error()
		run.ErrorMessage = &msg
		s.finalize(ctx, run)
		return
	}

	// Mirror the execution's summary onto the run so the runs list renders
	// without a join; the full tool-call trace stays on the execution record.
	run.ExecutionID = &exec.ID
	run.Output = exec.Output
	run.ErrorMessage = exec.ErrorMessage
	run.TotalTokens = exec.TotalTokens
	run.CostUSD = exec.CostUSD
	run.LatencyMs = exec.LatencyMs
	run.Iterations = exec.Iterations
	if exec.Status == model.ExecutionStatusFailed || exec.ErrorMessage != nil {
		run.Status = model.TaskRunStatusFailed
	} else {
		run.Status = model.TaskRunStatusCompleted
	}
	s.finalize(ctx, run)
}

func (s *taskRunService) finalize(ctx context.Context, run *model.TaskRun) {
	if err := s.runRepo.Update(ctx, run); err != nil {
		s.logger.Error("failed to persist terminal task run",
			zap.String("run_id", run.ID.String()),
			zap.String("status", run.Status),
			zap.Error(err),
		)
	}
	s.syncBoardStatus(ctx, run.TaskID, run.Status, nil)
}

// boardStatusForRun maps a generic task-run status onto the board column.
// Failed runs stay where they are (in_progress after start) so the card does
// not pretend the work is finished.
func boardStatusForRun(runStatus string) (taskStatus string, ok bool) {
	switch runStatus {
	case model.TaskRunStatusQueued, model.TaskRunStatusRunning:
		return "in_progress", true
	case model.TaskRunStatusCompleted:
		return "done", true
	default:
		return "", false
	}
}

func (s *taskRunService) syncBoardStatus(ctx context.Context, taskID uuid.UUID, runStatus string, changedBy *uuid.UUID) {
	status, ok := boardStatusForRun(runStatus)
	if !ok {
		return
	}
	if status == "done" {
		active, err := s.runRepo.CountActiveByTask(ctx, taskID)
		if err != nil {
			s.logger.Warn("failed to count active task runs before done sync",
				zap.String("task_id", taskID.String()),
				zap.Error(err),
			)
			return
		}
		if active > 0 {
			return
		}
	}
	if err := s.taskRepo.TransitionStatus(ctx, taskID, status, changedBy); err != nil {
		s.logger.Warn("failed to sync board status from task run",
			zap.String("task_id", taskID.String()),
			zap.String("run_status", runStatus),
			zap.Error(err),
		)
	}
}

func (s *taskRunService) GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.TaskRun, error) {
	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, ErrTaskRunNotFound
	}
	if run.OrgID != orgID {
		return nil, ErrTaskRunNotFound
	}
	return run, nil
}

func (s *taskRunService) ListRuns(ctx context.Context, taskID, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.TaskRun], error) {
	// Confirm the task belongs to the caller's org before listing its runs, so a
	// task ID from another org cannot be probed through the runs list.
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return nil, ErrTaskRunNotFound
	}
	project, err := s.projectRepo.FindByID(ctx, task.ProjectID)
	if err != nil || project == nil || project.OrgID != orgID {
		return nil, ErrTaskRunNotFound
	}
	pagination.Normalize()
	return s.runRepo.ListByTask(ctx, taskID, pagination)
}

// resolveRunAgent picks the agent for a run: the explicit override if present,
// otherwise the task's assigned agent. A task with neither is an error the
// caller surfaces as a 400.
func resolveRunAgent(override, assigned *uuid.UUID) (uuid.UUID, error) {
	if override != nil && *override != uuid.Nil {
		return *override, nil
	}
	if assigned != nil && *assigned != uuid.Nil {
		return *assigned, nil
	}
	return uuid.Nil, fmt.Errorf("no agent to run this task: assign an agent or pass agent_id")
}

// buildRunPrompt derives the prompt for a run: an explicit prompt override wins
// over the task's title+description, and any key/value inputs are appended as a
// context block regardless.
func buildRunPrompt(task *model.Task, req model.CreateTaskRunRequest) string {
	var b strings.Builder
	if req.Prompt != nil && strings.TrimSpace(*req.Prompt) != "" {
		b.WriteString(strings.TrimSpace(*req.Prompt))
	} else {
		b.WriteString(strings.TrimSpace(task.Title))
		if task.Description != nil && strings.TrimSpace(*task.Description) != "" {
			b.WriteString("\n\n")
			b.WriteString(strings.TrimSpace(*task.Description))
		}
	}
	if len(req.Inputs) > 0 {
		b.WriteString("\n\n## Inputs\n")
		// Deterministic order so re-runs with the same inputs read identically.
		keys := make([]string, 0, len(req.Inputs))
		for k := range req.Inputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("- %s: %v\n", k, req.Inputs[k]))
		}
	}
	return strings.TrimSpace(b.String())
}

// normalizeSlugs trims, lowercases and de-duplicates skill slugs, dropping
// blanks, so the stored set is clean and the executor lookup is stable.
func normalizeSlugs(slugs []string) []string {
	if len(slugs) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(slugs))
	out := make([]string, 0, len(slugs))
	for _, s := range slugs {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
