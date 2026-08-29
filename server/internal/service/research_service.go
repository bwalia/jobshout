package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
)

// ResearchService is the platform-wide "go find out about X" capability.
//
// It exists as its own service rather than as a private helper inside the blog
// pipeline because the capability is not specific to articles. Anything that
// needs current, cited material about a subject — an article, a report, a
// briefing for another agent — consumes the same typed research.Brief. The
// article pipeline is its first caller, not its owner.
type ResearchService interface {
	// Research plans, searches, reads and verifies, returning a brief whose
	// every finding is backed by a source that was actually retrieved.
	Research(ctx context.Context, orgID uuid.UUID, req research.Request, progress research.ProgressFunc) (*research.Brief, error)
	// Run is Research plus its bookkeeping: it returns the persisted run
	// alongside the brief, for callers that need to reference the work later —
	// a board task, a mail thread, or a client polling for completion.
	Run(ctx context.Context, orgID uuid.UUID, req research.Request, progress research.ProgressFunc, opts ResearchRunOptions) (*ResearchOutcome, error)
	// StartAsync records a run and researches in the background, returning as
	// soon as there is something to poll.
	StartAsync(ctx context.Context, orgID uuid.UUID, req research.Request, opts ResearchRunOptions) (*model.ResearchRun, error)
	// GetRun resolves a previously recorded run, scoped to the organisation.
	GetRun(ctx context.Context, orgID, runID uuid.UUID) (*model.ResearchRun, error)
	// ListRuns returns the organisation's research history, most recent first.
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.ResearchRun], error)
	// Trending reports what is currently being published across technology, AI
	// and infrastructure, for callers that need a subject rather than research
	// on one they already have.
	Trending(ctx context.Context, limit int) ([]research.TrendingItem, error)
	// Discover turns what is trending into subjects worth writing about,
	// skipping anything in avoid. This is the raw material a scheduled run
	// works from when nobody supplied a topic.
	Discover(ctx context.Context, orgID uuid.UUID, req research.DiscoverRequest, progress research.ProgressFunc) ([]research.Topic, error)
	// EnsureResearcher resolves the org's built-in Research Agent, creating it
	// if missing, so research runs are attributed and visible on the board.
	EnsureResearcher(ctx context.Context, orgID uuid.UUID) (*model.Agent, error)
	// Available reports whether research can run at all, so a caller can fail
	// early with a clear message instead of part-way through a pipeline.
	Available() bool
}

// ResearchRunOptions is the bookkeeping around a run: who asked for it, from
// which surface, and which board task it belongs to. None of it changes what
// the agent does — it changes what can be said about the run afterwards.
type ResearchRunOptions struct {
	// Source is one of the model.ResearchSource* constants.
	Source string
	// TaskID ties the run to a board task, when it was started from one.
	TaskID *uuid.UUID
	// RequestedBy is the user who asked, for attribution.
	RequestedBy *uuid.UUID
}

// ResearchOutcome pairs the persisted run with the typed brief.
//
// Both are returned because they answer different questions: the brief is the
// research, and the run is how anything else refers to it later. Run is nil
// when no repository is wired, which is the degraded-but-working case.
type ResearchOutcome struct {
	Run   *model.ResearchRun
	Brief *research.Brief
}

type researchService struct {
	agent     *research.Agent
	sources   *research.Client
	agentRepo repository.AgentRepository
	runs      repository.ResearchRunRepository
	logger    *zap.Logger
}

// NewResearchService wires the service. agent may be nil — with no LLM
// configured, research is unavailable and Available reports so. runs may be nil,
// in which case research still runs but leaves no record.
func NewResearchService(
	agent *research.Agent,
	sources *research.Client,
	agentRepo repository.AgentRepository,
	runs repository.ResearchRunRepository,
	logger *zap.Logger,
) ResearchService {
	return &researchService{
		agent: agent, sources: sources, agentRepo: agentRepo, runs: runs, logger: logger,
	}
}

func (s *researchService) GetRun(ctx context.Context, orgID, runID uuid.UUID) (*model.ResearchRun, error) {
	if s.runs == nil {
		return nil, fmt.Errorf("research_svc: research runs are not persisted on this server")
	}
	run, err := s.runs.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	// A run belonging to another organisation is reported as absent rather than
	// forbidden, so the API does not confirm that the id exists.
	if run == nil || run.OrgID != orgID {
		return nil, nil
	}
	return run, nil
}

func (s *researchService) ListRuns(ctx context.Context, orgID uuid.UUID, p model.PaginationParams) (*model.PaginatedResponse[model.ResearchRun], error) {
	if s.runs == nil {
		return &model.PaginatedResponse[model.ResearchRun]{Data: []model.ResearchRun{}}, nil
	}
	return s.runs.ListByOrg(ctx, orgID, p)
}

func (s *researchService) Available() bool {
	return s != nil && s.agent != nil && s.sources != nil
}

// researcherSeed is the built-in agent definition, mirroring articleWriterSeed.
func researcherSeed(orgID uuid.UUID) *model.Agent {
	desc := "Searches the internet, reads the sources it finds, and returns verified findings with citations that have been checked against the pages they came from."
	prompt := "You are a research agent. You plan searches, read sources in full, and extract only claims the source text actually states — each with the verbatim passage supporting it. You never cite a page you have not read, and you reject a citation when you are unsure it supports the claim."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "Research Agent",
		Role:         "Researcher",
		Description:  &desc,
		SystemPrompt: &prompt,
		// 'active' for the same reason as the Article Writer: the dashboard's
		// Active Agents grid filters on this and the agent is always available.
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinResearcher},
	}
}

func (s *researchService) EnsureResearcher(ctx context.Context, orgID uuid.UUID) (*model.Agent, error) {
	existing, err := s.agentRepo.FindBuiltin(ctx, orgID, model.BuiltinResearcher)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	agent := researcherSeed(orgID)
	if err := s.agentRepo.Create(ctx, agent); err != nil {
		return nil, fmt.Errorf("research_svc: seed research agent: %w", err)
	}
	s.logger.Info("research: seeded built-in Research Agent",
		zap.String("org_id", orgID.String()), zap.String("agent_id", agent.ID.String()))
	return agent, nil
}

func (s *researchService) Research(
	ctx context.Context,
	orgID uuid.UUID,
	req research.Request,
	progress research.ProgressFunc,
) (*research.Brief, error) {
	out, err := s.Run(ctx, orgID, req, progress, ResearchRunOptions{Source: model.ResearchSourceAPI})
	if err != nil {
		return nil, err
	}
	return out.Brief, nil
}

// Run performs research and records it as a research_runs row.
//
// Research delegates here, so every entry point leaves a trace whether or not
// it cares about the run: the board reads those rows, and until they existed
// the Research Agent could not appear on it at all.
//
// The run row is bookkeeping, and bookkeeping never fails the work. A
// repository that is absent or erroring degrades to the previous behaviour —
// research still runs and the brief is still returned — because trading a
// completed brief for an unwritable audit record is a bad exchange.
func (s *researchService) Run(
	ctx context.Context,
	orgID uuid.UUID,
	req research.Request,
	progress research.ProgressFunc,
	opts ResearchRunOptions,
) (*ResearchOutcome, error) {
	if !s.Available() {
		return nil, fmt.Errorf("research_svc: research is not configured (an LLM provider must be reachable)")
	}

	// Seeding is best-effort. It exists so the run is attributed on the board;
	// failing the research because a display record could not be written would
	// trade the actual work for its bookkeeping.
	agent, err := s.EnsureResearcher(ctx, orgID)
	if err != nil {
		s.logger.Warn("research: could not resolve the built-in agent, continuing unattributed",
			zap.Error(err))
	}

	run := s.startRun(ctx, orgID, agent, req, opts, model.ResearchRunRunning)

	brief, err := s.execute(ctx, run, req, progress)
	if err != nil {
		return nil, err
	}

	if agent != nil {
		s.logger.Info("research: brief complete",
			zap.String("agent_id", agent.ID.String()),
			zap.String("topic", req.Topic),
			zap.Int("findings", len(brief.Findings)),
			zap.Int("sources", len(brief.Sources)),
			zap.Int("warnings", len(brief.Warnings)),
		)
	}
	return &ResearchOutcome{Run: run, Brief: brief}, nil
}

// StartAsync records a run and performs the research in the background,
// returning as soon as the row exists so the caller can poll it.
//
// This is what replaces holding a browser request open for the length of a
// research run. It requires a repository: without somewhere to write the run
// there is nothing to poll, and returning an id that resolves to nothing would
// be worse than refusing.
func (s *researchService) StartAsync(
	ctx context.Context,
	orgID uuid.UUID,
	req research.Request,
	opts ResearchRunOptions,
) (*model.ResearchRun, error) {
	if !s.Available() {
		return nil, fmt.Errorf("research_svc: research is not configured (an LLM provider must be reachable)")
	}
	if s.runs == nil {
		return nil, fmt.Errorf("research_svc: research runs are not persisted on this server, so there is nothing to poll")
	}

	agent, err := s.EnsureResearcher(ctx, orgID)
	if err != nil {
		s.logger.Warn("research: could not resolve the built-in agent, continuing unattributed",
			zap.Error(err))
	}

	run := s.startRun(ctx, orgID, agent, req, opts, model.ResearchRunQueued)
	if run == nil {
		return nil, fmt.Errorf("research_svc: could not record the run")
	}

	go func() {
		// Detached from the request: the caller has already been answered, and
		// cancelling the research when they disconnect is precisely the
		// behaviour this endpoint exists to remove. The timeout is the backstop
		// so a wedged run cannot occupy a goroutine forever.
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), researchAsyncTimeout)
		defer cancel()
		if _, err := s.execute(bg, run, req, nil); err != nil {
			s.logger.Warn("research: background run failed",
				zap.String("run_id", run.ID.String()), zap.Error(err))
		}
	}()

	return run, nil
}

// researchAsyncTimeout bounds a background run. Generous because a brief means
// several model calls plus a handful of page fetches, and short enough that a
// stuck run is eventually recorded as failed rather than running forever.
const researchAsyncTimeout = 15 * time.Minute

// execute runs the agent and closes the run row out, either way.
func (s *researchService) execute(
	ctx context.Context,
	run *model.ResearchRun,
	req research.Request,
	progress research.ProgressFunc,
) (*research.Brief, error) {
	// Phase updates ride on the caller's progress callback rather than a second
	// mechanism, so the board and a live UI trace always agree.
	tracked := s.trackPhase(ctx, run, progress)
	s.markRunning(ctx, run)

	brief, err := s.agent.Research(ctx, req, tracked)
	if err != nil {
		s.finishRun(ctx, run, nil, err)
		return nil, err
	}
	s.finishRun(ctx, run, brief, nil)
	return brief, nil
}

// markRunning moves a queued run into running once work actually begins.
func (s *researchService) markRunning(ctx context.Context, run *model.ResearchRun) {
	if run == nil || s.runs == nil || run.Status == model.ResearchRunRunning {
		return
	}
	now := time.Now().UTC()
	run.Status = model.ResearchRunRunning
	run.StartedAt = &now
	if err := s.runs.Update(ctx, run); err != nil {
		s.logger.Debug("research: could not mark the run running", zap.Error(err))
	}
}

// startRun writes the run row. It returns nil when there is no repository, and
// every helper below tolerates a nil run for that reason.
func (s *researchService) startRun(
	ctx context.Context,
	orgID uuid.UUID,
	agent *model.Agent,
	req research.Request,
	opts ResearchRunOptions,
	status string,
) *model.ResearchRun {
	if s.runs == nil {
		return nil
	}
	run := &model.ResearchRun{
		OrgID:       orgID,
		TaskID:      opts.TaskID,
		RequestedBy: opts.RequestedBy,
		Source:      opts.Source,
		Topic:       req.Topic,
		Context:     req.Context,
		URLs:        req.URLs,
		Status:      status,
	}
	if status == model.ResearchRunRunning {
		now := time.Now().UTC()
		run.StartedAt = &now
	}
	if agent != nil {
		id := agent.ID
		run.AgentID = &id
	}
	if err := s.runs.Create(ctx, run); err != nil {
		s.logger.Warn("research: could not record the run, continuing untracked", zap.Error(err))
		return nil
	}
	return run
}

// trackPhase wraps the caller's progress function so each phase transition is
// also written to the run row.
func (s *researchService) trackPhase(
	ctx context.Context,
	run *model.ResearchRun,
	progress research.ProgressFunc,
) research.ProgressFunc {
	if run == nil || s.runs == nil {
		return progress
	}
	return func(phase, detail string) {
		if progress != nil {
			progress(phase, detail)
		}
		// Deliberately uses the request context: if the caller has gone away
		// the run is being abandoned anyway, and a phase write is not worth a
		// detached context of its own.
		if err := s.runs.UpdatePhase(ctx, run.ID, phase); err != nil {
			s.logger.Debug("research: phase not recorded", zap.Error(err))
		}
	}
}

// finishRun closes the run row out, either way.
func (s *researchService) finishRun(
	ctx context.Context,
	run *model.ResearchRun,
	brief *research.Brief,
	cause error,
) {
	if run == nil || s.runs == nil {
		return
	}
	now := time.Now().UTC()
	run.CompletedAt = &now
	run.Phase = ""

	if cause != nil {
		msg := cause.Error()
		run.Status = model.ResearchRunFailed
		run.ErrorMessage = &msg
	} else {
		run.Status = model.ResearchRunCompleted
		run.Usable = brief.IsUsable()
		if blob, err := json.Marshal(brief); err == nil {
			run.Brief = blob
		} else {
			s.logger.Warn("research: brief could not be serialised for the run row", zap.Error(err))
		}
	}
	if err := s.runs.Update(ctx, run); err != nil {
		s.logger.Warn("research: could not close out the run row", zap.Error(err))
	}
}

func (s *researchService) Discover(
	ctx context.Context,
	orgID uuid.UUID,
	req research.DiscoverRequest,
	progress research.ProgressFunc,
) ([]research.Topic, error) {
	if !s.Available() {
		return nil, fmt.Errorf("research_svc: topic discovery is not configured (an LLM provider must be reachable)")
	}
	// Best-effort, for the same reason as Research: attribution should not be
	// able to fail the work it is describing.
	if _, err := s.EnsureResearcher(ctx, orgID); err != nil {
		s.logger.Warn("research: could not resolve the built-in agent for discovery", zap.Error(err))
	}

	topics, err := s.agent.Discover(ctx, req, progress)
	if err != nil {
		return nil, err
	}
	s.logger.Info("research: discovery complete",
		zap.Int("topics", len(topics)), zap.Int("avoided", len(req.Avoid)))
	return topics, nil
}

func (s *researchService) Trending(ctx context.Context, limit int) ([]research.TrendingItem, error) {
	if s.sources == nil {
		return nil, fmt.Errorf("research_svc: research sources are not configured")
	}
	return s.sources.Trending(ctx, limit)
}
