package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	integ "github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ApprovalService drives human-in-the-loop approvals for gated agent tool calls.
// It satisfies executor.ApprovalGate (RequiresApproval + CreatePending) so it can
// be injected directly into the executor, and it owns the approve/reject flow
// that resumes a paused execution.
type ApprovalService interface {
	// executor.ApprovalGate
	RequiresApproval(agentID uuid.UUID, toolName string) bool
	CreatePending(ctx context.Context, execID, agentID, orgID uuid.UUID, toolName string, toolInput map[string]any, resumeState []byte) (uuid.UUID, error)

	List(ctx context.Context, orgID uuid.UUID, status string) ([]model.Approval, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Approval, error)
	// Decide records a human's approve/reject decision and, for a still-pending
	// approval, resumes the paused execution to completion (or the next gate),
	// persisting the outcome.
	Decide(ctx context.Context, id, deciderID uuid.UUID, decision, reason string) (*model.Approval, error)
}

// Ensure the concrete type satisfies the executor's narrow gate interface.
var _ executor.ApprovalGate = (*approvalService)(nil)

type approvalService struct {
	repo      repository.ApprovalRepository
	execRepo  repository.ExecutionRepository
	agentRepo repository.AgentRepository
	userRepo  repository.UserRepository
	exec      *executor.Executor
	notifSvc  NotificationService
	logger    *zap.Logger
}

// NewApprovalService creates an ApprovalService.
// agentRepo, userRepo and notifSvc may be nil; when set they enrich manager
// notifications and rejection messages. exec is required to resume paused runs.
func NewApprovalService(
	repo repository.ApprovalRepository,
	execRepo repository.ExecutionRepository,
	agentRepo repository.AgentRepository,
	userRepo repository.UserRepository,
	exec *executor.Executor,
	notifSvc NotificationService,
	logger *zap.Logger,
) ApprovalService {
	return &approvalService{
		repo:      repo,
		execRepo:  execRepo,
		agentRepo: agentRepo,
		userRepo:  userRepo,
		exec:      exec,
		notifSvc:  notifSvc,
		logger:    logger,
	}
}

// RequiresApproval reports whether the tool is gated for the agent. It is
// best-effort and default-off: any lookup error yields false so the gate never
// breaks an execution.
func (s *approvalService) RequiresApproval(agentID uuid.UUID, toolName string) bool {
	gated, err := s.repo.IsGated(context.Background(), agentID, toolName)
	if err != nil {
		s.logger.Warn("approval gate lookup failed; treating tool as ungated",
			zap.String("agent_id", agentID.String()),
			zap.String("tool", toolName),
			zap.Error(err),
		)
		return false
	}
	return gated
}

func (s *approvalService) CreatePending(ctx context.Context, execID, agentID, orgID uuid.UUID, toolName string, toolInput map[string]any, resumeState []byte) (uuid.UUID, error) {
	a := &model.Approval{
		ID:          uuid.New(),
		OrgID:       orgID,
		ExecutionID: execID,
		AgentID:     agentID,
		ToolName:    toolName,
		ToolInput:   toolInput,
		Status:      model.ApprovalStatusPending,
		ResumeState: resumeState,
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return uuid.Nil, fmt.Errorf("approval_svc: create pending: %w", err)
	}

	// Best-effort: notify the agent's human manager that a decision is needed.
	s.notifyManager(ctx, a)

	return a.ID, nil
}

func (s *approvalService) List(ctx context.Context, orgID uuid.UUID, status string) ([]model.Approval, error) {
	return s.repo.ListByOrg(ctx, orgID, status)
}

func (s *approvalService) Get(ctx context.Context, id uuid.UUID) (*model.Approval, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *approvalService) Decide(ctx context.Context, id, deciderID uuid.UUID, decision, reason string) (*model.Approval, error) {
	approval, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("approval_svc: find: %w", err)
	}
	if approval.Status != model.ApprovalStatusPending {
		return nil, fmt.Errorf("approval_svc: approval %s already decided (%s)", id, approval.Status)
	}

	status := model.ApprovalStatusApproved
	if decision == "reject" {
		status = model.ApprovalStatusRejected
	}

	if err := s.repo.UpdateDecision(ctx, id, status, reason, deciderID); err != nil {
		return nil, fmt.Errorf("approval_svc: update decision: %w", err)
	}

	// Reload so we resume from the persisted, decided record (with resume_state).
	approval, err = s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("approval_svc: reload: %w", err)
	}
	approval.DeciderName = s.deciderName(ctx, deciderID)

	// Resume the paused execution. If the resumed run hits another gate, the
	// executor records a fresh pending approval via this same service, so we
	// must NOT persist that as a completed result — leave the execution running.
	result := s.exec.Resume(ctx, approval)
	if result.Status == executor.StatusAwaitingApproval {
		s.logger.Info("resumed execution paused again on a further gate",
			zap.String("execution_id", approval.ExecutionID.String()),
			zap.String("next_approval_id", result.ApprovalID.String()),
		)
		return approval, nil
	}

	if err := s.execRepo.PersistResult(ctx, approval.ExecutionID, result); err != nil {
		s.logger.Error("failed to persist resumed execution result",
			zap.String("execution_id", approval.ExecutionID.String()),
			zap.Error(err),
		)
	}

	return approval, nil
}

// notifyManager dispatches a best-effort notification about a pending approval.
// It never returns an error and never blocks the caller meaningfully — a missing
// notification service or a dispatch failure is logged and swallowed.
func (s *approvalService) notifyManager(ctx context.Context, a *model.Approval) {
	if s.notifSvc == nil {
		return
	}

	title := fmt.Sprintf("Approval required: agent tool %q", a.ToolName)
	status := "pending"
	if s.agentRepo != nil {
		if agent, err := s.agentRepo.FindByID(ctx, a.AgentID); err == nil && agent != nil {
			title = fmt.Sprintf("Approval required: %s wants to run %q", agent.Name, a.ToolName)
			if agent.ManagerID != nil {
				status = "awaiting manager " + agent.ManagerID.String()
			}
		}
	}

	event := integ.TaskEvent{
		Type:   integ.EventApprovalRequested,
		OrgID:  a.OrgID,
		Title:  title,
		Status: status,
	}
	if err := s.notifSvc.DispatchEvent(ctx, a.OrgID, event); err != nil {
		s.logger.Warn("failed to dispatch approval notification",
			zap.String("approval_id", a.ID.String()),
			zap.Error(err),
		)
	}
}

// deciderName resolves a best-effort human-readable name for the decider, used
// only to make rejection messages readable. Falls back to the empty string.
func (s *approvalService) deciderName(ctx context.Context, deciderID uuid.UUID) string {
	if s.userRepo == nil {
		return ""
	}
	user, err := s.userRepo.FindByID(ctx, deciderID)
	if err != nil || user == nil {
		return ""
	}
	if user.FullName != "" {
		return user.FullName
	}
	return user.Email
}
