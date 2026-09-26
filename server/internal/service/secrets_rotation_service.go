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

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/secretsrot"
)

// ErrSecretsRotationRunNotFound is returned when a run is missing or wrong org.
var ErrSecretsRotationRunNotFound = errors.New("secrets rotation run not found")

// ErrSecretsRotationNotCancellable is returned for terminal runs.
var ErrSecretsRotationNotCancellable = errors.New("secrets rotation run cannot be cancelled")

// SecretsRotationService is the launch and HTTP surface.
type SecretsRotationService interface {
	CreateRun(ctx context.Context, req model.CreateSecretsRotationRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.SecretsRotationRun, error)
	GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.SecretsRotationRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.SecretsRotationRun], error)
	CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.SecretsRotationRun, error)
	Status(ctx context.Context) map[string]any
}

type secretsRotationService struct {
	repo      repository.SecretsRotationRunRepository
	agentRepo repository.AgentRepository
	cfg       secretsrot.Config
	logger    *zap.Logger

	mu      sync.Mutex
	cancels map[uuid.UUID]context.CancelFunc
}

// NewSecretsRotationService wires persistence + vault config.
func NewSecretsRotationService(
	repo repository.SecretsRotationRunRepository,
	agentRepo repository.AgentRepository,
	cfg secretsrot.Config,
	logger *zap.Logger,
) SecretsRotationService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &secretsRotationService{
		repo:      repo,
		agentRepo: agentRepo,
		cfg:       cfg,
		logger:    logger,
		cancels:   make(map[uuid.UUID]context.CancelFunc),
	}
}

func (s *secretsRotationService) Status(ctx context.Context) map[string]any {
	_ = ctx
	return map[string]any{
		"enabled":      s.cfg.Enabled(),
		"vault_addr":   s.cfg.Addr,
		"has_token":    s.cfg.Token != "",
		"namespace":    s.cfg.Namespace != "",
		"ui_login_url": secretsrot.DefaultUILoginURL,
		"docs_url":     secretsrot.DefaultDocsURL,
		// Propagation readiness — presence only, never values.
		"kubeconfig_dir":    s.cfg.KubeconfigDir,
		"has_github_token":  s.cfg.GitHubToken != "",
		"rp_url":            s.cfg.RPURL,
		"has_rp_token":      s.cfg.RPToken != "",
		"has_rp_prod_login": s.cfg.RPProdPassword != "",
		"message": func() string {
			if s.cfg.Enabled() {
				return "Vault token configured — ready to plan/rotate"
			}
			return "Set SECRETS_VAULT_TOKEN (or VAULT_TOKEN) and optional SECRETS_VAULT_ADDR"
		}(),
	}
}

func (s *secretsRotationService) CreateRun(ctx context.Context, req model.CreateSecretsRotationRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.SecretsRotationRun, error) {
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
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = "auto"
	}
	engine := strings.TrimSpace(req.Engine)
	if engine == "" {
		engine = "kv2"
	}
	mount := strings.TrimSpace(req.Mount)
	if mount == "" {
		mount = "secret"
	}
	grace := req.GraceSeconds
	if grace < 0 {
		grace = 0
	}
	addr := strings.TrimSpace(req.VaultAddr)
	if addr == "" {
		addr = s.cfg.Addr
	}
	prop, err := secretsrot.ParsePropagation(propagationInput(req.Source, req.Cluster, req.ExternalSecrets,
		req.K8sSecrets, req.GitHubRepo, req.GitHubEnvironment, req.GitHubSecretNames, req.RPApp, req.RPRings, req.RPDeployments))
	if err != nil {
		return nil, err
	}
	if err := secretsrot.ValidateLaunch(mode, engine, secretsrot.ParseKeys(req.Keys), prop); err != nil {
		return nil, err
	}
	var instruction *string
	if strings.TrimSpace(req.Instruction) != "" {
		i := strings.TrimSpace(req.Instruction)
		instruction = &i
	}
	now := time.Now()
	run := &model.SecretsRotationRun{
		ID:           uuid.New(),
		AgentID:      req.AgentID,
		TaskID:       req.TaskID,
		OrgID:        orgID,
		Status:       "queued",
		Mode:         mode,
		Provider:     provider,
		VaultAddr:    addr,
		Mount:        mount,
		Path:         strings.TrimSpace(req.Path),
		Engine:       engine,
		Keys:         strings.TrimSpace(req.Keys),
		GraceSeconds: grace,
		RetireOld:    req.RetireOld,
		DryRun:       req.DryRun,
		Instruction:  instruction,
		RequestedBy:  requestedBy,

		Source:            prop.Source,
		Cluster:           prop.Cluster,
		ExternalSecrets:   strings.TrimSpace(req.ExternalSecrets),
		K8sSecrets:        strings.TrimSpace(req.K8sSecrets),
		GitHubRepo:        prop.GitHubRepo,
		GitHubEnvironment: prop.GitHubEnv,
		GitHubSecretNames: strings.TrimSpace(req.GitHubSecretNames),
		RPApp:             prop.RPApp,
		RPRings:           strings.TrimSpace(req.RPRings),
		RPDeployments:     strings.TrimSpace(req.RPDeployments),
		StartedAt:         &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.Create(ctx, run); err != nil {
		return nil, err
	}

	newSecret, err := secretsrot.ParseNewSecretJSON(req.NewSecretJSON)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[run.ID] = cancel
	s.mu.Unlock()
	go s.execute(runCtx, run, newSecret)

	return run, nil
}

func (s *secretsRotationService) execute(ctx context.Context, run *model.SecretsRotationRun, newSecret map[string]any) {
	defer func() {
		s.mu.Lock()
		delete(s.cancels, run.ID)
		s.mu.Unlock()
	}()

	run.Status = "running"
	_ = s.repo.Update(ctx, run)

	opt := secretsrot.RunOptions{
		Mode:         run.Mode,
		Provider:     run.Provider,
		VaultAddr:    run.VaultAddr,
		Mount:        run.Mount,
		Path:         run.Path,
		Engine:       run.Engine,
		Keys:         secretsrot.ParseKeys(run.Keys),
		GraceSeconds: run.GraceSeconds,
		RetireOld:    run.RetireOld,
		DryRun:       run.DryRun,
		NewSecret:    newSecret,
		RunID:        run.ID.String(),
	}
	// Validated in CreateRun; a stored row always parses.
	if prop, err := secretsrot.ParsePropagation(propagationInput(run.Source, run.Cluster, run.ExternalSecrets,
		run.K8sSecrets, run.GitHubRepo, run.GitHubEnvironment, run.GitHubSecretNames, run.RPApp, run.RPRings, run.RPDeployments)); err == nil {
		opt.Propagate = prop
	} else {
		msg := err.Error()
		run.ErrorMessage = &msg
		run.Status = "failed"
		now := time.Now()
		run.CompletedAt = &now
		_ = s.repo.Update(context.Background(), run)
		return
	}

	outcome, err := secretsrot.Execute(ctx, s.cfg, opt, func(p model.SecretsRotationPhase) {
		// Replace matching phase in-place for live UI polling.
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
		run.DetectedProv = outcome.DetectedProvider
		res := outcome.Result
		run.Result = &res
	}
	if err != nil {
		msg := err.Error()
		run.ErrorMessage = &msg
		run.Status = "failed"
		now := time.Now()
		run.CompletedAt = &now
		_ = s.repo.Update(context.Background(), run)
		return
	}
	if ctx.Err() != nil {
		run.Status = "cancelled"
		msg := "cancelled"
		run.ErrorMessage = &msg
		now := time.Now()
		run.CompletedAt = &now
		_ = s.repo.Update(context.Background(), run)
		return
	}
	now := time.Now()
	run.Status = "completed"
	run.CompletedAt = &now
	if err := s.repo.Update(context.Background(), run); err != nil {
		s.logger.Error("secrets rotation update failed", zap.Error(err))
	}
}

func (s *secretsRotationService) GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.SecretsRotationRun, error) {
	run, err := s.repo.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, ErrSecretsRotationRunNotFound
	}
	return run, nil
}

func (s *secretsRotationService) ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.SecretsRotationRun], error) {
	pagination.Normalize()
	return s.repo.ListByOrg(ctx, orgID, pagination)
}

func (s *secretsRotationService) CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.SecretsRotationRun, error) {
	run, err := s.GetRun(ctx, runID, orgID)
	if err != nil {
		return nil, err
	}
	switch run.Status {
	case "cancelled":
		return run, nil
	case "completed", "failed":
		return nil, ErrSecretsRotationNotCancellable
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

func propagationInput(source, cluster, externalSecrets, k8sSecrets, ghRepo, ghEnv, ghNames, rpApp, rpRings, rpDeployments string) secretsrot.PropagationInput {
	return secretsrot.PropagationInput{
		Source:            source,
		Cluster:           cluster,
		ExternalSecrets:   externalSecrets,
		K8sSecrets:        k8sSecrets,
		GitHubRepo:        ghRepo,
		GitHubEnvironment: ghEnv,
		GitHubNames:       ghNames,
		RPApp:             rpApp,
		RPRings:           rpRings,
		RPDeployments:     rpDeployments,
	}
}
