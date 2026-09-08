package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/waflab"
)

// ErrWAFLabRunNotFound is returned when a run is missing or belongs to another org.
var ErrWAFLabRunNotFound = errors.New("waf lab run not found")

// ErrWAFLabNotConfigured is returned when wslproxy credentials are not set.
var ErrWAFLabNotConfigured = errors.New("waf efficacy lab is not configured")

// ErrWAFLabNotCancellable is returned for terminal runs.
var ErrWAFLabNotCancellable = errors.New("waf lab run cannot be cancelled")

// WAFLabService is the launch and HTTP surface for the WAF Efficacy Lab.
type WAFLabService interface {
	Enabled() bool
	CreateRun(ctx context.Context, req model.CreateWAFLabRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.WAFLabRun, error)
	GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.WAFLabRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.WAFLabRun], error)
	ListSteps(ctx context.Context, runID, orgID uuid.UUID) ([]model.WAFLabStep, error)
	ListResults(ctx context.Context, runID, orgID uuid.UUID) ([]model.WAFLabResult, error)
	CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.WAFLabRun, error)
	Status() map[string]any
}

type wafLabService struct {
	repo      repository.WAFLabRunRepository
	agentRepo repository.AgentRepository
	cfg       waflab.Config
	client    *waflab.Client
	logger    *zap.Logger

	mu      sync.Mutex
	cancels map[uuid.UUID]context.CancelFunc
}

// NewWAFLabService wires persistence and the wslproxy client.
func NewWAFLabService(
	repo repository.WAFLabRunRepository,
	agentRepo repository.AgentRepository,
	cfg waflab.Config,
	client *waflab.Client,
	logger *zap.Logger,
) WAFLabService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &wafLabService{
		repo:      repo,
		agentRepo: agentRepo,
		cfg:       cfg,
		client:    client,
		logger:    logger,
		cancels:   make(map[uuid.UUID]context.CancelFunc),
	}
}

func (s *wafLabService) Enabled() bool {
	return s != nil && s.client != nil && s.client.Enabled()
}

func (s *wafLabService) Status() map[string]any {
	return map[string]any{
		"enabled":               s.Enabled(),
		"wslproxy_base_url":     s.cfg.BaseURL,
		"cloudflare_configured": waflab.CloudflareConfigured(),
		"platform":              s.cfg.Platform,
		"profile":               s.cfg.Profile,
		"lab_max_runtime":       s.cfg.LabMaxRuntime.String(),
	}
}

func (s *wafLabService) CreateRun(ctx context.Context, req model.CreateWAFLabRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.WAFLabRun, error) {
	if !s.Enabled() {
		return nil, ErrWAFLabNotConfigured
	}
	if err := validateCreateReq(req); err != nil {
		return nil, err
	}

	agent, err := s.agentRepo.FindByID(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found")
	}
	if agent.OrgID != orgID {
		return nil, fmt.Errorf("agent does not belong to organization")
	}

	now := time.Now()
	run := &model.WAFLabRun{
		ID:              uuid.New(),
		AgentID:         req.AgentID,
		TaskID:          req.TaskID,
		OrgID:           orgID,
		Status:          "queued",
		Mode:            req.Mode,
		SecureHost:      strings.TrimSpace(req.SecureHost),
		OpenHost:        strings.TrimSpace(req.OpenHost),
		OriginUpstream:  defaultStr(req.OriginUpstream, "127.0.0.1:30084"),
		PolicyID:        defaultStr(req.PolicyID, "waf-policy-payments-hard"),
		ManageDNS:       defaultStr(req.ManageDNS, "off"),
		DNSZone:         strings.TrimSpace(req.DNSZone),
		AttackSet:       defaultStr(req.AttackSet, "full"),
		WSLProxyBaseURL: strings.TrimRight(strings.TrimSpace(req.WSLProxyBaseURL), "/"),
		RequestedBy:     requestedBy,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if note := strings.TrimSpace(req.Instruction); note != "" {
		run.Instruction = &note
	}

	if err := s.repo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create waf lab run: %w", err)
	}

	maxRT := s.cfg.LabMaxRuntime
	if maxRT <= 0 {
		maxRT = 10 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(context.Background(), maxRT)
	s.mu.Lock()
	s.cancels[run.ID] = cancel
	s.mu.Unlock()

	go s.execute(runCtx, cancel, run)

	s.logger.Info("waf lab run queued", zap.String("runID", run.ID.String()))
	return run, nil
}

func (s *wafLabService) execute(ctx context.Context, cancel context.CancelFunc, run *model.WAFLabRun) {
	defer cancel()
	defer func() {
		s.mu.Lock()
		delete(s.cancels, run.ID)
		s.mu.Unlock()
	}()

	now := time.Now()
	run.Status = "running"
	run.StartedAt = &now
	if err := s.repo.Update(context.Background(), run); err != nil {
		s.logger.Error("waf lab mark running failed", zap.String("runID", run.ID.String()), zap.Error(err))
	}

	client := s.clientFor(run.WSLProxyBaseURL)
	cfg := waflab.LabConfig{
		SecureHost:     run.SecureHost,
		OpenHost:       run.OpenHost,
		OriginUpstream: run.OriginUpstream,
		PolicyID:       run.PolicyID,
		Mode:           run.Mode,
		ManageDNS:      run.ManageDNS,
		DNSZone:        run.DNSZone,
		AttackSet:      run.AttackSet,
		ProfileID:      s.cfg.Profile,
		CloudflareOK:   waflab.CloudflareConfigured(),
	}
	if run.Instruction != nil {
		cfg.Instruction = *run.Instruction
	}

	rec := &liveStepRecorder{repo: s.repo, runID: run.ID, logger: s.logger}
	labOut, labErr := waflab.RunLab(ctx, client, cfg, rec)

	done := time.Now()
	run.CompletedAt = &done

	// Steps are already persisted live via liveStepRecorder; Finalize only
	// needs results + terminal run state so we do not double-insert phases.
	results := make([]model.WAFLabResult, 0)
	var score *model.WAFLabScore

	if labOut != nil {
		hostFor := map[string]string{"secure": run.SecureHost, "open": run.OpenHost}
		for _, hr := range labOut.Results {
			host := hr.Host
			if host == "" {
				host = hostFor[hr.HostRole]
			}
			results = append(results, model.WAFLabResult{
				ID:           uuid.New(),
				RunID:        run.ID,
				HostRole:     hr.HostRole,
				Host:         host,
				AttackID:     hr.AttackID,
				AttackName:   hr.AttackName,
				Category:     hr.Category,
				Method:       hr.Method,
				Path:         hr.Path,
				Payload:      hr.Payload,
				StatusCode:   hr.StatusCode,
				Blocked:      hr.Blocked,
				ExpectBlock:  hr.ExpectBlock,
				WAFRule:      hr.WAFRule,
				WAFViolation: hr.WAFViolation,
				SupportID:    hr.SupportID,
				LatencyMS:    hr.LatencyMS,
				Verdict:      waflab.VerdictFor(hr),
				Notes:        hr.Notes,
			})
		}
		sc := labOut.Score
		score = &sc
	}

	if ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
		// Prefer cancelled if CancelRun already flipped the row; otherwise mark cancelled.
		current, _ := s.repo.GetByID(context.Background(), run.ID)
		if current != nil && current.Status == "cancelled" {
			return
		}
		run.Status = "cancelled"
		msg := "cancelled"
		run.ErrorMessage = &msg
	} else if labErr != nil {
		current, _ := s.repo.GetByID(context.Background(), run.ID)
		if current != nil && current.Status == "cancelled" {
			return
		}
		run.Status = "failed"
		msg := labErr.Error()
		run.ErrorMessage = &msg
	} else {
		current, _ := s.repo.GetByID(context.Background(), run.ID)
		if current != nil && current.Status == "cancelled" {
			return
		}
		run.Status = "completed"
	}

	if err := s.repo.Finalize(context.Background(), run, nil, results, score); err != nil {
		s.logger.Error("waf lab finalize failed", zap.String("runID", run.ID.String()), zap.Error(err))
		_ = s.repo.Update(context.Background(), run)
	}
}

func (s *wafLabService) clientFor(baseURL string) *waflab.Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || baseURL == s.cfg.BaseURL {
		return s.client
	}
	cfg := s.cfg
	cfg.BaseURL = baseURL
	return waflab.NewClient(cfg, s.logger)
}

func (s *wafLabService) GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.WAFLabRun, error) {
	run, err := s.repo.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, ErrWAFLabRunNotFound
	}
	return run, nil
}

func (s *wafLabService) ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.WAFLabRun], error) {
	pagination.Normalize()
	return s.repo.ListByOrg(ctx, orgID, pagination)
}

func (s *wafLabService) ListSteps(ctx context.Context, runID, orgID uuid.UUID) ([]model.WAFLabStep, error) {
	if _, err := s.GetRun(ctx, runID, orgID); err != nil {
		return nil, err
	}
	return s.repo.ListSteps(ctx, runID)
}

func (s *wafLabService) ListResults(ctx context.Context, runID, orgID uuid.UUID) ([]model.WAFLabResult, error) {
	if _, err := s.GetRun(ctx, runID, orgID); err != nil {
		return nil, err
	}
	return s.repo.ListResults(ctx, runID)
}

func (s *wafLabService) CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.WAFLabRun, error) {
	run, err := s.GetRun(ctx, runID, orgID)
	if err != nil {
		return nil, err
	}
	switch run.Status {
	case "cancelled":
		return run, nil
	case "completed", "failed":
		return nil, ErrWAFLabNotCancellable
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
		return nil, fmt.Errorf("failed to cancel waf lab run: %w", err)
	}
	return run, nil
}

type liveStepRecorder struct {
	repo   repository.WAFLabRunRepository
	runID  uuid.UUID
	logger *zap.Logger
}

func (r *liveStepRecorder) RecordStep(phase, status, message string) {
	now := time.Now()
	step := &model.WAFLabStep{
		ID:        uuid.New(),
		RunID:     r.runID,
		Phase:     phase,
		Status:    status,
		Message:   message,
		StartedAt: &now,
		EndedAt:   &now,
	}
	if err := r.repo.CreateStep(context.Background(), step); err != nil && r.logger != nil {
		r.logger.Warn("waf lab step persist failed", zap.String("phase", phase), zap.Error(err))
	}
}

func validateCreateReq(req model.CreateWAFLabRunRequest) error {
	base := strings.TrimSpace(req.WSLProxyBaseURL)
	if base == "" {
		return fmt.Errorf("wslproxy_base_url is required")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("wslproxy_base_url must be https")
	}
	if strings.TrimSpace(req.SecureHost) == "" {
		return fmt.Errorf("secure_host is required")
	}
	if strings.TrimSpace(req.OpenHost) == "" {
		return fmt.Errorf("open_host is required")
	}
	switch req.Mode {
	case "provision_and_test", "provision_only", "test_only":
	case "":
		return fmt.Errorf("mode is required")
	default:
		return fmt.Errorf("invalid mode %q", req.Mode)
	}
	return nil
}

func defaultStr(v, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}
