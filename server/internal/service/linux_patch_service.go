package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/linuxpatch"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/secretsrot"
)

var (
	ErrLinuxPatchRunNotFound     = errors.New("linux patch run not found")
	ErrLinuxPatchNotCancellable  = errors.New("linux patch run cannot be cancelled")
)

// LinuxPatchService is the launch and HTTP surface.
type LinuxPatchService interface {
	CreateRun(ctx context.Context, req model.CreateLinuxPatchRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.LinuxPatchRun, error)
	GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.LinuxPatchRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.LinuxPatchRun], error)
	CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.LinuxPatchRun, error)
	Status(ctx context.Context) map[string]any
}

type linuxPatchService struct {
	repo      repository.LinuxPatchRunRepository
	agentRepo repository.AgentRepository
	cfg       linuxpatch.Config
	vaultCfg  secretsrot.Config
	llm       llm.Client
	logger    *zap.Logger
	mu        sync.Mutex
	cancels   map[uuid.UUID]context.CancelFunc
}

// NewLinuxPatchService wires SSH + optional Vault + LLM.
func NewLinuxPatchService(
	repo repository.LinuxPatchRunRepository,
	agentRepo repository.AgentRepository,
	cfg linuxpatch.Config,
	vaultCfg secretsrot.Config,
	llmClient llm.Client,
	logger *zap.Logger,
) LinuxPatchService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &linuxPatchService{
		repo: repo, agentRepo: agentRepo, cfg: cfg, vaultCfg: vaultCfg, llm: llmClient, logger: logger,
		cancels: make(map[uuid.UUID]context.CancelFunc),
	}
}

func (s *linuxPatchService) Status(ctx context.Context) map[string]any {
	_ = ctx
	return map[string]any{
		"ssh_auth_configured": s.cfg.HasAuth(),
		"vault_configured":    s.vaultCfg.Enabled(),
		"vault_addr":          s.vaultCfg.Addr,
		"llm_configured":      s.llm != nil,
		"ui_login_url":        secretsrot.DefaultUILoginURL,
		"docs_url":            linuxpatch.WSLVaultDocs,
		"message": func() string {
			switch {
			case s.cfg.HasAuth() && s.vaultCfg.Enabled():
				return "SSH env + Vault ready — workflows can chain Secrets Rotation → Linux Patch"
			case s.vaultCfg.Enabled():
				return "Vault ready — set vault_path on runs to fetch SSH keys"
			case s.cfg.HasAuth():
				return "SSH env auth ready"
			default:
				return "Set LINUX_PATCH_SSH_KEY or vault_path + SECRETS_VAULT_TOKEN"
			}
		}(),
	}
}

func (s *linuxPatchService) CreateRun(ctx context.Context, req model.CreateLinuxPatchRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.LinuxPatchRun, error) {
	agent, err := s.agentRepo.FindByID(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil || agent.OrgID != orgID {
		return nil, fmt.Errorf("agent not found")
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "plan"
	}
	workload := strings.TrimSpace(req.Workload)
	if workload == "" {
		workload = "generic"
	}
	reboot := strings.TrimSpace(req.RebootPolicy)
	if reboot == "" {
		reboot = "if_needed"
	}
	var instruction *string
	if strings.TrimSpace(req.Instruction) != "" {
		i := strings.TrimSpace(req.Instruction)
		instruction = &i
	}
	var schedule *string
	if strings.TrimSpace(req.ScheduleAfter) != "" {
		sc := strings.TrimSpace(req.ScheduleAfter)
		schedule = &sc
	}
	now := time.Now()
	run := &model.LinuxPatchRun{
		ID: uuid.New(), AgentID: req.AgentID, TaskID: req.TaskID, OrgID: orgID,
		Status: "queued", Mode: mode, Workload: workload, Hosts: strings.TrimSpace(req.Hosts),
		SSHUser: strings.TrimSpace(req.SSHUser), PreScript: req.PreScript, PostScript: req.PostScript,
		Services: strings.TrimSpace(req.Services), RebootPolicy: reboot, DryRun: req.DryRun, UseLLM: req.UseLLM,
		ScheduleAfter: schedule, VaultMount: strings.TrimSpace(req.VaultMount), VaultPath: strings.TrimSpace(req.VaultPath),
		VaultKeyField: strings.TrimSpace(req.VaultKeyField), Instruction: instruction, RequestedBy: requestedBy,
		StartedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, run); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[run.ID] = cancel
	s.mu.Unlock()
	go s.execute(runCtx, run)
	return run, nil
}

func (s *linuxPatchService) execute(ctx context.Context, run *model.LinuxPatchRun) {
	defer func() {
		s.mu.Lock()
		delete(s.cancels, run.ID)
		s.mu.Unlock()
	}()
	run.Status = "running"
	_ = s.repo.Update(ctx, run)

	opt := linuxpatch.RunOptions{
		Mode: run.Mode, Workload: run.Workload, HostsRaw: run.Hosts, SSHUser: run.SSHUser,
		PreScript: run.PreScript, PostScript: run.PostScript, Services: linuxpatch.ParseServices(run.Services),
		RebootPolicy: run.RebootPolicy, DryRun: run.DryRun, UseLLM: run.UseLLM,
		VaultMount: run.VaultMount, VaultPath: run.VaultPath, VaultKeyField: run.VaultKeyField,
		VaultCfg: s.vaultCfg,
	}
	if run.Instruction != nil {
		opt.Instruction = *run.Instruction
	}
	var llmClient llm.Client
	if run.UseLLM {
		llmClient = s.llm
	}
	outcome, err := linuxpatch.Execute(ctx, s.cfg, llmClient, opt, func(p model.LinuxPatchPhase) {
		found := false
		for i := range run.Phases {
			if run.Phases[i].Key == p.Key {
				run.Phases[i] = p
				found = true
				break
			}
		}
		if !found {
			run.Phases = append(run.Phases, p)
		}
		_ = s.repo.Update(context.Background(), run)
	})
	if outcome != nil {
		run.Phases = outcome.Phases
		run.HostResults = outcome.HostResults
		plan := outcome.Plan
		run.Plan = &plan
	}
	now := time.Now()
	run.CompletedAt = &now
	if err != nil {
		msg := err.Error()
		run.ErrorMessage = &msg
		run.Status = "failed"
	} else if ctx.Err() != nil {
		run.Status = "cancelled"
		msg := "cancelled"
		run.ErrorMessage = &msg
	} else {
		run.Status = "completed"
	}
	_ = s.repo.Update(context.Background(), run)
}

func (s *linuxPatchService) GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.LinuxPatchRun, error) {
	run, err := s.repo.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, ErrLinuxPatchRunNotFound
	}
	return run, nil
}

func (s *linuxPatchService) ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.LinuxPatchRun], error) {
	pagination.Normalize()
	return s.repo.ListByOrg(ctx, orgID, pagination)
}

func (s *linuxPatchService) CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.LinuxPatchRun, error) {
	run, err := s.GetRun(ctx, runID, orgID)
	if err != nil {
		return nil, err
	}
	switch run.Status {
	case "cancelled":
		return run, nil
	case "completed", "failed":
		return nil, ErrLinuxPatchNotCancellable
	}
	s.mu.Lock()
	if cancel, ok := s.cancels[runID]; ok {
		cancel()
	}
	s.mu.Unlock()
	now := time.Now()
	run.Status = "cancelled"
	run.CompletedAt = &now
	msg := "cancelled by operator"
	run.ErrorMessage = &msg
	if err := s.repo.Update(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}
