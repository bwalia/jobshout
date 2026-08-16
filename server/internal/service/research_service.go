package service

import (
	"context"
	"fmt"

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

type researchService struct {
	agent     *research.Agent
	sources   *research.Client
	agentRepo repository.AgentRepository
	logger    *zap.Logger
}

// NewResearchService wires the service. agent may be nil — with no LLM
// configured, research is unavailable and Available reports so.
func NewResearchService(
	agent *research.Agent,
	sources *research.Client,
	agentRepo repository.AgentRepository,
	logger *zap.Logger,
) ResearchService {
	return &researchService{agent: agent, sources: sources, agentRepo: agentRepo, logger: logger}
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

	brief, err := s.agent.Research(ctx, req, progress)
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
	return brief, nil
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
