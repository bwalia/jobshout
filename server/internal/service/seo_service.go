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
	"github.com/jobshout/server/internal/seo"
)

// ErrSEORunNotFound is returned when a run is missing or belongs to another org.
var ErrSEORunNotFound = errors.New("seo run not found")

// ErrSEONotCancellable is returned for terminal runs.
var ErrSEONotCancellable = errors.New("seo run cannot be cancelled")

// SEOService is the launch and HTTP surface for the SEO Analyst.
type SEOService interface {
	CreateRun(ctx context.Context, req model.CreateSEORunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.SEORun, error)
	GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.SEORun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.SEORun], error)
	CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.SEORun, error)
}

type seoService struct {
	repo      repository.SEORunRepository
	agentRepo repository.AgentRepository
	logger    *zap.Logger

	mu      sync.Mutex
	cancels map[uuid.UUID]context.CancelFunc
}

// NewSEOService wires persistence.
func NewSEOService(repo repository.SEORunRepository, agentRepo repository.AgentRepository, logger *zap.Logger) SEOService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &seoService{
		repo:      repo,
		agentRepo: agentRepo,
		logger:    logger,
		cancels:   make(map[uuid.UUID]context.CancelFunc),
	}
}

func (s *seoService) CreateRun(ctx context.Context, req model.CreateSEORunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.SEORun, error) {
	agent, err := s.agentRepo.FindByID(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil || agent.OrgID != orgID {
		return nil, fmt.Errorf("agent not found")
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "analyze"
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "auto"
	}
	branch := strings.TrimSpace(req.GitBranch)
	if branch == "" {
		branch = "main"
	}
	now := time.Now()
	var instruction *string
	if strings.TrimSpace(req.Instruction) != "" {
		i := strings.TrimSpace(req.Instruction)
		instruction = &i
	}
	run := &model.SEORun{
		ID:          uuid.New(),
		AgentID:     req.AgentID,
		TaskID:      req.TaskID,
		OrgID:       orgID,
		Status:      "queued",
		Mode:        mode,
		URL:         strings.TrimSpace(req.URL),
		Platform:    platform,
		GitRepo:     strings.TrimSpace(req.GitRepo),
		GitBranch:   branch,
		Keywords:    strings.TrimSpace(req.Keywords),
		Instruction: instruction,
		RequestedBy: requestedBy,
		StartedAt:   &now,
		CreatedAt:   now,
		UpdatedAt:   now,
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

func (s *seoService) execute(ctx context.Context, run *model.SEORun) {
	defer func() {
		s.mu.Lock()
		delete(s.cancels, run.ID)
		s.mu.Unlock()
	}()

	run.Status = "running"
	_ = s.repo.Update(ctx, run)

	bundle, err := seo.Analyze(ctx, seo.AnalyzeOptions{
		URL:      run.URL,
		Keywords: seo.ParseKeywords(run.Keywords),
	})
	if err != nil {
		s.fail(run, err.Error())
		return
	}
	if ctx.Err() != nil {
		s.cancel(run)
		return
	}

	run.DetectedCMS = bundle.DetectedCMS
	score := model.SEOScore{
		Overall: bundle.Score.Overall, TitleOK: bundle.Score.TitleOK, MetaDescOK: bundle.Score.MetaDescOK,
		H1OK: bundle.Score.H1OK, CanonicalOK: bundle.Score.CanonicalOK, RobotsOK: bundle.Score.RobotsOK,
		SitemapOK: bundle.Score.SitemapOK, OpenGraphOK: bundle.Score.OpenGraphOK, HTTPSOK: bundle.Score.HTTPSOK,
		ByCategory: bundle.Score.ByCategory, IssueCount: bundle.Score.IssueCount, CriticalCount: bundle.Score.CriticalCount,
	}
	run.Score = &score

	issues := make([]model.SEOIssue, 0, len(bundle.Issues))
	for _, i := range bundle.Issues {
		issues = append(issues, model.SEOIssue{
			ID: i.ID, Severity: i.Severity, Category: i.Category, Title: i.Title, Detail: i.Detail, FixHint: i.FixHint,
		})
	}
	suggestions := make([]model.SEOSuggestion, 0, len(bundle.Suggestions))
	for _, sg := range bundle.Suggestions {
		suggestions = append(suggestions, model.SEOSuggestion{
			ID: sg.ID, Kind: sg.Kind, Title: sg.Title, Detail: sg.Detail, Proposed: sg.Proposed,
		})
	}
	report := &model.SEOReport{
		FinalURL: bundle.FinalURL, StatusCode: bundle.StatusCode, Title: bundle.Title,
		MetaDescription: bundle.MetaDescription, Canonical: bundle.Canonical,
		H1: bundle.H1, H2: bundle.H2, KeywordsFound: bundle.KeywordsFound,
		RobotsTxt: bundle.RobotsTxt, SitemapURL: bundle.SitemapURL, SitemapFound: bundle.SitemapFound,
		OpenGraph: bundle.OpenGraph, Issues: issues, Suggestions: suggestions,
		ProposedSlug: bundle.ProposedSlug, SitemapXML: bundle.SitemapXML, MetaPatch: bundle.MetaPatch,
	}

	// Site structure skills (static / git)
	if run.GitRepo != "" || run.Mode != "analyze" {
		st := seo.InferSiteStructure(nil, "")
		report.SiteStructure = &model.SEOSiteStructure{
			ContentRoot: st.ContentRoot, PublicRoot: st.PublicRoot, TemplateHint: st.TemplateHint,
			Pages: st.Pages, HasSitemap: st.HasSitemap || bundle.SitemapFound, SkillNotes: st.SkillNotes,
		}
	}
	run.Report = report

	if run.Mode == "analyze" {
		s.complete(run)
		return
	}

	// improve already populated MetaPatch / SitemapXML / Suggestions
	if run.Mode == "improve" {
		s.complete(run)
		return
	}

	// publish
	platform := run.Platform
	if platform == "auto" {
		platform = bundle.DetectedCMS
	}
	var structure *seo.SiteStructure
	if report.SiteStructure != nil {
		structure = &seo.SiteStructure{
			ContentRoot: report.SiteStructure.ContentRoot, PublicRoot: report.SiteStructure.PublicRoot,
			TemplateHint: report.SiteStructure.TemplateHint, Pages: report.SiteStructure.Pages,
			HasSitemap: report.SiteStructure.HasSitemap, SkillNotes: report.SiteStructure.SkillNotes,
		}
	}
	pub := seo.Publish(ctx, seo.PublishInput{
		SiteURL:     report.FinalURL,
		Platform:    platform,
		DetectedCMS: bundle.DetectedCMS,
		GitRepo:     run.GitRepo,
		GitBranch:   run.GitBranch,
		SitemapPath: "sitemap.xml",
		SitemapXML:  report.SitemapXML,
		MetaPatch:   report.MetaPatch,
		ProposedSlug: report.ProposedSlug,
		Keywords:    seo.ParseKeywords(run.Keywords),
		Structure:   structure,
	})
	run.Publish = &model.SEOPublishResult{
		Channel: pub.Channel, Success: pub.Success, Message: pub.Message,
		PRURL: pub.PRURL, PRNumber: pub.PRNumber, RemoteRef: pub.RemoteRef, Fallback: pub.Fallback,
	}
	if !pub.Success && run.Mode == "publish" {
		msg := pub.Message
		run.ErrorMessage = &msg
		// Still mark completed with publish failure details — analysis succeeded.
	}
	s.complete(run)
}

func (s *seoService) complete(run *model.SEORun) {
	now := time.Now()
	run.Status = "completed"
	run.CompletedAt = &now
	if err := s.repo.Update(context.Background(), run); err != nil {
		s.logger.Error("seo run update failed", zap.Error(err))
	}
}

func (s *seoService) fail(run *model.SEORun, msg string) {
	now := time.Now()
	run.Status = "failed"
	run.ErrorMessage = &msg
	run.CompletedAt = &now
	_ = s.repo.Update(context.Background(), run)
}

func (s *seoService) cancel(run *model.SEORun) {
	now := time.Now()
	run.Status = "cancelled"
	msg := "cancelled"
	run.ErrorMessage = &msg
	run.CompletedAt = &now
	_ = s.repo.Update(context.Background(), run)
}

func (s *seoService) GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.SEORun, error) {
	run, err := s.repo.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, ErrSEORunNotFound
	}
	return run, nil
}

func (s *seoService) ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.SEORun], error) {
	pagination.Normalize()
	return s.repo.ListByOrg(ctx, orgID, pagination)
}

func (s *seoService) CancelRun(ctx context.Context, runID, orgID uuid.UUID) (*model.SEORun, error) {
	run, err := s.GetRun(ctx, runID, orgID)
	if err != nil {
		return nil, err
	}
	switch run.Status {
	case "cancelled":
		return run, nil
	case "completed", "failed":
		return nil, ErrSEONotCancellable
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
