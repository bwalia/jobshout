package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	integ "github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ErrApprovalNotFound is returned when an approval does not exist (or, at the
// handler, belongs to another org).
var ErrApprovalNotFound = errors.New("approval not found")

// ErrApprovalAlreadyDecided is returned by Decide when the approval has already
// been approved or rejected — including the loser of two concurrent decides.
var ErrApprovalAlreadyDecided = errors.New("approval already decided")

const (
	// defaultResumeTimeout bounds one background resume: the rest of the agent
	// run after a decision. It is the resume's own budget, not the request's.
	defaultResumeTimeout = 15 * time.Minute
	// defaultResumeStaleAfter is how old a resume claim must be before another
	// worker may take it over (its worker is presumed dead). It must exceed
	// defaultResumeTimeout so a live resume is never run twice.
	defaultResumeStaleAfter = 2 * defaultResumeTimeout
	// persistTimeout bounds writing a resume's outcome, which runs on a fresh
	// context so a resume that hit its own timeout is still recorded.
	persistTimeout = 30 * time.Second
	// recoveryBatch caps how many unfinished resumes one recovery pass picks up.
	recoveryBatch = 50
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
	// Get returns the approval, or ErrApprovalNotFound.
	Get(ctx context.Context, id uuid.UUID) (*model.Approval, error)
	// Decide records a human's approve/reject decision and returns at once
	// with the decided approval; the paused execution resumes in the
	// background on its own context and timeout, and its outcome is written
	// to the execution and to the task run that launched it. Deciding twice
	// returns ErrApprovalAlreadyDecided; an unknown id ErrApprovalNotFound.
	Decide(ctx context.Context, id, deciderID uuid.UUID, decision, reason string) (*model.Approval, error)
	// RunRecovery resumes decided approvals whose resume never finished (the
	// server stopped mid-resume), now and then every interval until ctx ends.
	RunRecovery(ctx context.Context, interval time.Duration)
}

// Ensure the concrete type satisfies the executor's narrow gate interface.
var _ executor.ApprovalGate = (*approvalService)(nil)

// approvalResumer continues a paused run; satisfied by *executor.Executor.
type approvalResumer interface {
	Resume(ctx context.Context, approval *model.Approval) executor.Result
}

// ExecutionRunSync mirrors an execution's state onto the task run that
// launched it; satisfied by TaskRunService.
type ExecutionRunSync interface {
	SyncFromExecution(ctx context.Context, exec *model.AgentExecution) error
}

type approvalService struct {
	repo      repository.ApprovalRepository
	execSvc   ExecutionService
	agentRepo repository.AgentRepository
	userRepo  repository.UserRepository
	exec      approvalResumer
	notifSvc  NotificationService
	runSync   ExecutionRunSync
	logger    *zap.Logger

	resumeTimeout time.Duration
	staleAfter    time.Duration
	// inflight tracks background resumes (tests wait on it).
	inflight sync.WaitGroup
}

// NewApprovalService creates an ApprovalService.
// agentRepo, userRepo, notifSvc and runSync may be nil; when set they enrich
// manager notifications and rejection messages, and keep task runs in step
// with resumed executions. exec and execSvc are required to resume paused runs.
func NewApprovalService(
	repo repository.ApprovalRepository,
	execSvc ExecutionService,
	agentRepo repository.AgentRepository,
	userRepo repository.UserRepository,
	exec *executor.Executor,
	notifSvc NotificationService,
	runSync ExecutionRunSync,
	logger *zap.Logger,
) ApprovalService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &approvalService{
		repo:          repo,
		execSvc:       execSvc,
		agentRepo:     agentRepo,
		userRepo:      userRepo,
		exec:          exec,
		notifSvc:      notifSvc,
		runSync:       runSync,
		logger:        logger,
		resumeTimeout: defaultResumeTimeout,
		staleAfter:    defaultResumeStaleAfter,
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
	a, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, repository.ErrApprovalNotFound) {
		return nil, ErrApprovalNotFound
	}
	return a, err
}

func (s *approvalService) Decide(ctx context.Context, id, deciderID uuid.UUID, decision, reason string) (*model.Approval, error) {
	approval, err := s.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrApprovalNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("approval_svc: find: %w", err)
	}
	if approval.Status != model.ApprovalStatusPending {
		return nil, ErrApprovalAlreadyDecided
	}

	status := model.ApprovalStatusApproved
	if decision == "reject" {
		status = model.ApprovalStatusRejected
	}

	// The update only matches a pending row, so of two racing decides exactly
	// one wins; the other gets ErrApprovalAlreadyDecided.
	if err := s.repo.UpdateDecision(ctx, id, status, reason, deciderID); err != nil {
		switch {
		case errors.Is(err, repository.ErrApprovalNotPending):
			return nil, ErrApprovalAlreadyDecided
		case errors.Is(err, repository.ErrApprovalNotFound):
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("approval_svc: update decision: %w", err)
	}

	// Reload so we resume from the persisted, decided record (with resume_state).
	approval, err = s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("approval_svc: reload: %w", err)
	}

	s.resumeInBackground(approval)
	return approval, nil
}

// resumeInBackground resumes a decided approval's run on its own context and
// timeout, detached from whoever asked — an HTTP request must not hold the
// rest of an agent run, nor cancel it by going away.
func (s *approvalService) resumeInBackground(approval *model.Approval) {
	s.inflight.Add(1)
	go func() {
		defer s.inflight.Done()
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("approval resume panicked",
					zap.String("approval_id", approval.ID.String()),
					zap.Any("panic", rec))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), s.resumeTimeout)
		defer cancel()
		s.resumeDecided(ctx, approval)
	}()
}

// resumeDecided drives one decided approval's paused run to its next state and
// writes that state to the execution and its task run. It is safe to call for
// the same approval from several workers or replicas: only the one that claims
// it resumes it.
func (s *approvalService) resumeDecided(ctx context.Context, approval *model.Approval) {
	log := s.logger.With(
		zap.String("approval_id", approval.ID.String()),
		zap.String("execution_id", approval.ExecutionID.String()),
	)

	claimed, err := s.repo.ClaimResume(ctx, approval.ID, s.staleAfter)
	if err != nil {
		log.Error("failed to claim approval for resume", zap.Error(err))
		return
	}
	if !claimed {
		log.Info("approval resume already claimed or finished; skipping")
		return
	}

	ok, err := s.execSvc.MarkResuming(ctx, approval.ExecutionID)
	if err != nil {
		// Leave the claim: recovery retries once it goes stale.
		log.Error("failed to mark execution resuming", zap.Error(err))
		return
	}
	if !ok {
		log.Info("execution is no longer paused (cancelled or finished); not resuming")
		s.markResumed(ctx, approval.ID, log)
		return
	}
	if exec, err := s.execSvc.GetByID(ctx, approval.ExecutionID); err == nil {
		s.syncRun(ctx, exec, log)
	}

	approval.DeciderName = s.deciderName(ctx, deriveDecider(approval))
	result := s.exec.Resume(ctx, approval)

	// The outcome is written on a fresh context: a resume that ran out of time
	// must still be recorded (as failed), not left looking like it runs on.
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()

	exec, err := s.execSvc.CompleteResumed(pctx, approval.ExecutionID, result)
	if err != nil {
		log.Error("failed to persist resumed execution result", zap.Error(err))
	} else {
		s.syncRun(pctx, exec, log)
	}
	s.markResumed(pctx, approval.ID, log)

	if result.Status == executor.StatusAwaitingApproval {
		log.Info("resumed execution paused again on a further gate",
			zap.String("next_approval_id", result.ApprovalID.String()))
	} else {
		log.Info("resumed execution finished", zap.Bool("failed", result.Err != nil))
	}
}

func (s *approvalService) syncRun(ctx context.Context, exec *model.AgentExecution, log *zap.Logger) {
	if s.runSync == nil || exec == nil {
		return
	}
	if err := s.runSync.SyncFromExecution(ctx, exec); err != nil {
		log.Warn("failed to sync task run from resumed execution", zap.Error(err))
	}
}

func (s *approvalService) markResumed(ctx context.Context, id uuid.UUID, log *zap.Logger) {
	if err := s.repo.MarkResumed(ctx, id); err != nil {
		log.Error("failed to mark approval resumed", zap.Error(err))
	}
}

func deriveDecider(a *model.Approval) uuid.UUID {
	if a.DecidedBy != nil {
		return *a.DecidedBy
	}
	return uuid.Nil
}

// RunRecovery resumes decided approvals whose resume never finished, at once
// and then every interval, until ctx is done.
func (s *approvalService) RunRecovery(ctx context.Context, interval time.Duration) {
	s.recoverUnresumed(ctx)
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.recoverUnresumed(ctx)
		}
	}
}

func (s *approvalService) recoverUnresumed(ctx context.Context) {
	list, err := s.repo.ListUnresumed(ctx, s.staleAfter, recoveryBatch)
	if err != nil {
		s.logger.Warn("approval recovery: list unresumed failed", zap.Error(err))
		return
	}
	for i := range list {
		a := list[i]
		s.logger.Info("approval recovery: resuming a decided approval whose resume never finished",
			zap.String("approval_id", a.ID.String()))
		s.resumeInBackground(&a)
	}
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
