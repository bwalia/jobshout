package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

	Status(ctx context.Context) map[string]any
	ListConnections(ctx context.Context, orgID uuid.UUID) ([]seo.ConnectionPublic, error)
	UpsertConnection(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, name, repo, branch, token string) (*seo.ConnectionPublic, error)
	DeleteConnection(ctx context.Context, orgID uuid.UUID, name string) error
}

type seoService struct {
	repo      repository.SEORunRepository
	agentRepo repository.AgentRepository
	conns     *seo.ConnStore
	cfg       seo.Config
	logger    *zap.Logger

	mu      sync.Mutex
	cancels map[uuid.UUID]context.CancelFunc
}

// NewSEOService wires persistence and optional GitHub connection store.
func NewSEOService(repo repository.SEORunRepository, agentRepo repository.AgentRepository, logger *zap.Logger) SEOService {
	return NewSEOServiceFull(repo, agentRepo, nil, seo.LoadConfig(), logger)
}

// NewSEOServiceFull wires runs + named GitHub connections.
func NewSEOServiceFull(repo repository.SEORunRepository, agentRepo repository.AgentRepository, pool *pgxpool.Pool, cfg seo.Config, logger *zap.Logger) SEOService {
	if logger == nil {
		logger = zap.NewNop()
	}
	var conns *seo.ConnStore
	if pool != nil {
		conns = seo.NewConnStore(pool)
	}
	return &seoService{
		repo:      repo,
		agentRepo: agentRepo,
		conns:     conns,
		cfg:       cfg,
		logger:    logger,
		cancels:   make(map[uuid.UUID]context.CancelFunc),
	}
}

func (s *seoService) Status(_ context.Context) map[string]any {
	return map[string]any{
		"can_seal_tokens": s.cfg.CanSealTokens(),
		"connections":     s.conns != nil,
	}
}

func (s *seoService) ListConnections(ctx context.Context, orgID uuid.UUID) ([]seo.ConnectionPublic, error) {
	if s.conns == nil {
		return nil, nil
	}
	rows, err := s.conns.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]seo.ConnectionPublic, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].Public())
	}
	return out, nil
}

func (s *seoService) UpsertConnection(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, name, repo, branch, token string) (*seo.ConnectionPublic, error) {
	if s.conns == nil {
		return nil, fmt.Errorf("github connections not configured")
	}
	if !s.cfg.CanSealTokens() {
		return nil, fmt.Errorf("SEO_TOKEN_KEY is not set — cannot store GitHub tokens")
	}
	name = strings.TrimSpace(name)
	repo = strings.TrimSpace(repo)
	token = strings.TrimSpace(token)
	if name == "" || repo == "" || token == "" {
		return nil, fmt.Errorf("name, repo, and access_token are required")
	}
	if _, _, err := parseOwnerRepo(repo); err != nil {
		return nil, err
	}
	enc, err := seo.SealToken(s.cfg.TokenKey, token)
	if err != nil {
		return nil, err
	}
	if branch == "" {
		branch = "main"
	}
	c := &seo.GitHubConnection{
		OrgID: orgID, Name: name, Repo: repo, DefaultBranch: branch,
		AccessTokenEnc: enc, ConnectedBy: userID,
	}
	if err := s.conns.Upsert(ctx, c); err != nil {
		return nil, err
	}
	pub := c.Public()
	return &pub, nil
}

func (s *seoService) DeleteConnection(ctx context.Context, orgID uuid.UUID, name string) error {
	if s.conns == nil {
		return fmt.Errorf("github connections not configured")
	}
	return s.conns.Delete(ctx, orgID, strings.TrimSpace(name))
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
	source := strings.ToLower(strings.TrimSpace(req.Source))
	if source == "" {
		if strings.TrimSpace(req.GitHubConnection) != "" {
			source = "github"
		} else {
			source = "urls"
		}
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "auto"
	}
	branch := strings.TrimSpace(req.GitBranch)
	if branch == "" {
		branch = "main"
	}
	maxPages := req.MaxPages
	if maxPages <= 0 {
		maxPages = 10
	}
	if maxPages > 50 {
		maxPages = 50
	}
	keywords := strings.TrimSpace(req.Keywords)
	if keywords == "" {
		return nil, fmt.Errorf("keywords are required — recommendations are driven by your keyword set")
	}

	urlField := strings.TrimSpace(req.URL)
	urlsRaw := strings.TrimSpace(req.URLs)
	if urlsRaw == "" && urlField != "" {
		urlsRaw = urlField
	}

	switch source {
	case "urls":
		if len(seo.ParseURLs(urlsRaw)) == 0 {
			return nil, fmt.Errorf("at least one URL is required for urls mode")
		}
		if urlField == "" {
			urlField = seo.ParseURLs(urlsRaw)[0]
		}
	case "github":
		if strings.TrimSpace(req.GitHubConnection) == "" {
			return nil, fmt.Errorf("github_connection is required for github mode (named server-side credential)")
		}
		if urlField == "" {
			urlField = "github:" + strings.TrimSpace(req.GitHubConnection)
		}
	default:
		return nil, fmt.Errorf("source must be urls or github")
	}

	now := time.Now()
	var instruction *string
	if strings.TrimSpace(req.Instruction) != "" {
		i := strings.TrimSpace(req.Instruction)
		instruction = &i
	}
	run := &model.SEORun{
		ID:               uuid.New(),
		AgentID:          req.AgentID,
		TaskID:           req.TaskID,
		OrgID:            orgID,
		Status:           "queued",
		Mode:             mode,
		Source:           source,
		URL:              urlField,
		Platform:         platform,
		GitRepo:          strings.TrimSpace(req.GitRepo),
		GitBranch:        branch,
		GitHubConnection: strings.TrimSpace(req.GitHubConnection),
		PathGlob:         strings.TrimSpace(req.PathGlob),
		MaxPages:         maxPages,
		Keywords:         keywords,
		Instruction:      instruction,
		RequestedBy:      requestedBy,
		StartedAt:        &now,
		CreatedAt:        now,
		UpdatedAt:        now,
		Progress:         &model.SEOProgress{Phase: "queued"},
	}
	// Stash multi-URL list on pages queue for execute (paths = URLs in urls mode).
	if source == "urls" {
		for _, u := range seo.ParseURLs(urlsRaw) {
			if len(run.Pages) >= maxPages {
				break
			}
			run.Pages = append(run.Pages, model.SEOPageReport{Path: u, SourceURL: u, Status: "queued"})
		}
		run.Progress.Total = len(run.Pages)
	}

	if err := s.repo.Create(ctx, run); err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[run.ID] = cancel
	s.mu.Unlock()
	go s.execute(runCtx, run, strings.TrimSpace(req.SiteMapPath))

	return run, nil
}

func (s *seoService) execute(ctx context.Context, run *model.SEORun, sitemapPath string) {
	defer func() {
		s.mu.Lock()
		delete(s.cancels, run.ID)
		s.mu.Unlock()
	}()

	run.Status = "running"
	run.Progress = &model.SEOProgress{Phase: "analysing", Total: len(run.Pages), Completed: 0}
	_ = s.repo.Update(ctx, run)

	keywords := seo.ParseKeywords(run.Keywords)
	var gitToken string

	if run.Source == "github" {
		pages, repo, token, err := s.listGitHubPages(ctx, run)
		if err != nil {
			s.fail(run, err.Error())
			return
		}
		gitToken = token
		if run.GitRepo == "" {
			run.GitRepo = repo
		}
		run.Pages = pages
		run.Progress = &model.SEOProgress{Phase: "analysing", Total: len(pages), Completed: 0}
		_ = s.repo.Update(ctx, run)
	}

	if len(run.Pages) == 0 {
		s.fail(run, "no pages to analyse")
		return
	}

	var allIssues []model.SEOIssue
	var allSuggestions []model.SEOSuggestion
	var metaPatch map[string]string
	var first *seo.ReportBundle
	filePatches := map[string]string{}
	scoreSum := 0
	scoreN := 0

	for i := range run.Pages {
		if ctx.Err() != nil {
			s.cancel(run)
			return
		}
		page := &run.Pages[i]
		page.Status = "running"
		run.Progress.Current = page.Path
		run.Progress.Completed = i
		_ = s.repo.Update(ctx, run)

		var bundle *seo.ReportBundle
		var err error
		if run.Source == "github" {
			bundle, err = s.analyseGitHubFile(ctx, run, gitToken, page.Path)
		} else {
			bundle, err = seo.Analyze(ctx, seo.AnalyzeOptions{URL: page.SourceURL, Keywords: keywords})
		}
		if err != nil {
			page.Status = "failed"
			page.Error = err.Error()
			continue
		}
		page.Status = "completed"
		page.Title = bundle.Title
		page.MetaDescription = bundle.MetaDescription
		page.H1 = bundle.H1
		page.KeywordsFound = bundle.KeywordsFound
		page.MetaPatch = bundle.MetaPatch
		pageScore := mapScore(bundle.Score)
		page.Score = &pageScore
		page.Issues = mapIssues(bundle.Issues)
		page.Suggestions = mapSuggestions(bundle.Suggestions)
		for _, sg := range bundle.Suggestions {
			if sg.Proposed != "" && (sg.ID == "frontmatter" || sg.ID == "html_meta") {
				page.ProposedDiff = sg.Proposed
				if run.Source == "github" && run.Mode != "analyze" {
					// Never overwrite source with a partial snippet — drop proposals under seo-patches/.
					safe := strings.ReplaceAll(page.Path, "/", "__")
					filePatches["seo-patches/"+safe+".md"] = "# Proposed SEO patch for `" + page.Path + "`\n\n```\n" + sg.Proposed + "\n```\n"
				}
			}
		}
		allIssues = append(allIssues, page.Issues...)
		allSuggestions = append(allSuggestions, page.Suggestions...)
		scoreSum += bundle.Score.Overall
		scoreN++
		if first == nil {
			first = bundle
			metaPatch = bundle.MetaPatch
			run.DetectedCMS = bundle.DetectedCMS
		}
		run.Progress.Completed = i + 1
		_ = s.repo.Update(ctx, run)
	}

	if first == nil {
		s.fail(run, "all pages failed to analyse")
		return
	}

	avg := scoreSum
	if scoreN > 0 {
		avg = scoreSum / scoreN
	}
	rollup := mapScore(first.Score)
	rollup.Overall = avg
	rollup.IssueCount = len(allIssues)
	critical := 0
	for _, iss := range allIssues {
		if iss.Severity == "critical" || iss.Severity == "high" {
			critical++
		}
	}
	rollup.CriticalCount = critical
	run.Score = &rollup

	kwFound := map[string]bool{}
	for _, p := range run.Pages {
		for _, k := range p.KeywordsFound {
			kwFound[k] = true
		}
	}
	foundList := make([]string, 0, len(kwFound))
	for k := range kwFound {
		foundList = append(foundList, k)
	}

	report := &model.SEOReport{
		FinalURL: first.FinalURL, StatusCode: first.StatusCode, Title: first.Title,
		MetaDescription: first.MetaDescription, Canonical: first.Canonical,
		H1: first.H1, H2: first.H2, KeywordsFound: foundList,
		RobotsTxt: first.RobotsTxt, SitemapURL: first.SitemapURL, SitemapFound: first.SitemapFound,
		OpenGraph: first.OpenGraph, Issues: allIssues, Suggestions: dedupeSuggestions(allSuggestions),
		ProposedSlug: first.ProposedSlug, SitemapXML: first.SitemapXML, MetaPatch: metaPatch,
		PageCount: len(run.Pages), Keywords: keywords,
		RollupSummary: fmt.Sprintf("Analysed %d page(s); average score %d; %d issue(s).", len(run.Pages), avg, len(allIssues)),
	}
	if run.GitRepo != "" || run.Source == "github" {
		paths := make([]string, 0, len(run.Pages))
		for _, p := range run.Pages {
			paths = append(paths, p.Path)
		}
		st := seo.InferSiteStructure(paths, "")
		report.SiteStructure = &model.SEOSiteStructure{
			ContentRoot: st.ContentRoot, PublicRoot: st.PublicRoot, TemplateHint: st.TemplateHint,
			Pages: st.Pages, HasSitemap: st.HasSitemap || first.SitemapFound, SkillNotes: st.SkillNotes,
		}
	}
	run.Report = report
	run.Progress.Phase = "done"
	run.Progress.Current = ""

	if run.Mode == "analyze" || run.Mode == "improve" {
		if run.Mode == "improve" && len(filePatches) > 0 && run.Publish == nil {
			// Surface suggested diffs without publishing.
			run.Publish = &model.SEOPublishResult{
				Channel: "suggested_diff",
				Success: true,
				Message: fmt.Sprintf("Proposed patches for %d file(s). Switch action to publish to open a PR.", len(filePatches)),
			}
		}
		s.complete(run)
		return
	}

	// publish
	run.Progress.Phase = "publishing"
	_ = s.repo.Update(ctx, run)

	platform := run.Platform
	if platform == "auto" {
		platform = first.DetectedCMS
	}
	var structure *seo.SiteStructure
	if report.SiteStructure != nil {
		structure = &seo.SiteStructure{
			ContentRoot: report.SiteStructure.ContentRoot, PublicRoot: report.SiteStructure.PublicRoot,
			TemplateHint: report.SiteStructure.TemplateHint, Pages: report.SiteStructure.Pages,
			HasSitemap: report.SiteStructure.HasSitemap, SkillNotes: report.SiteStructure.SkillNotes,
		}
	}
	if sitemapPath == "" {
		sitemapPath = "sitemap.xml"
	}
	pub := seo.Publish(ctx, seo.PublishInput{
		SiteURL: report.FinalURL, Platform: platform, DetectedCMS: first.DetectedCMS,
		GitRepo: run.GitRepo, GitBranch: run.GitBranch, GitToken: gitToken,
		SitemapPath: sitemapPath, SitemapXML: report.SitemapXML, MetaPatch: report.MetaPatch,
		ProposedSlug: report.ProposedSlug, Keywords: keywords, FilePatches: filePatches, Structure: structure,
	})
	run.Publish = &model.SEOPublishResult{
		Channel: pub.Channel, Success: pub.Success, Message: pub.Message,
		PRURL: pub.PRURL, PRNumber: pub.PRNumber, RemoteRef: pub.RemoteRef, Fallback: pub.Fallback,
	}
	if !pub.Success {
		msg := pub.Message
		run.ErrorMessage = &msg
	}
	run.Progress.Phase = "done"
	s.complete(run)
}

func (s *seoService) listGitHubPages(ctx context.Context, run *model.SEORun) ([]model.SEOPageReport, string, string, error) {
	if s.conns == nil {
		return nil, "", "", fmt.Errorf("github connections not configured")
	}
	conn, err := s.conns.GetByName(ctx, run.OrgID, run.GitHubConnection)
	if err != nil {
		return nil, "", "", err
	}
	token, err := seo.OpenToken(s.cfg.TokenKey, conn.AccessTokenEnc)
	if err != nil {
		return nil, "", "", fmt.Errorf("unseal token: %w", err)
	}
	repo := run.GitRepo
	if repo == "" {
		repo = conn.Repo
	}
	branch := run.GitBranch
	if branch == "" {
		branch = conn.DefaultBranch
	}
	owner, name, err := parseOwnerRepo(repo)
	if err != nil {
		return nil, "", "", err
	}
	client := &seo.RepoClient{Token: token}
	run.Progress = &model.SEOProgress{Phase: "listing"}
	_ = s.repo.Update(ctx, run)
	paths, err := client.ListPaths(ctx, owner, name, branch)
	if err != nil {
		return nil, "", "", err
	}
	matched := seo.MatchGlob(paths, run.PathGlob, run.MaxPages)
	if len(matched) == 0 {
		return nil, "", "", fmt.Errorf("no matching pages for glob %q (looked for .html/.md/.mdx)", run.PathGlob)
	}
	pages := make([]model.SEOPageReport, 0, len(matched))
	for _, p := range matched {
		pages = append(pages, model.SEOPageReport{Path: p, Status: "queued"})
	}
	return pages, repo, token, nil
}

func (s *seoService) analyseGitHubFile(ctx context.Context, run *model.SEORun, token, path string) (*seo.ReportBundle, error) {
	repo := run.GitRepo
	owner, name, err := parseOwnerRepo(repo)
	if err != nil {
		return nil, err
	}
	client := &seo.RepoClient{Token: token}
	content, err := client.GetFile(ctx, owner, name, run.GitBranch, path)
	if err != nil {
		return nil, err
	}
	return seo.AnalyzeContent(seo.AnalyzeContentOptions{
		Path: path, Content: content, Keywords: seo.ParseKeywords(run.Keywords),
	}), nil
}

func mapScore(s seo.Score) model.SEOScore {
	return model.SEOScore{
		Overall: s.Overall, TitleOK: s.TitleOK, MetaDescOK: s.MetaDescOK,
		H1OK: s.H1OK, CanonicalOK: s.CanonicalOK, RobotsOK: s.RobotsOK,
		SitemapOK: s.SitemapOK, OpenGraphOK: s.OpenGraphOK, HTTPSOK: s.HTTPSOK,
		ByCategory: s.ByCategory, IssueCount: s.IssueCount, CriticalCount: s.CriticalCount,
	}
}

func mapIssues(in []seo.Issue) []model.SEOIssue {
	out := make([]model.SEOIssue, 0, len(in))
	for _, i := range in {
		out = append(out, model.SEOIssue{
			ID: i.ID, Severity: i.Severity, Category: i.Category, Title: i.Title, Detail: i.Detail, FixHint: i.FixHint,
		})
	}
	return out
}

func mapSuggestions(in []seo.Suggestion) []model.SEOSuggestion {
	out := make([]model.SEOSuggestion, 0, len(in))
	for _, sg := range in {
		out = append(out, model.SEOSuggestion{
			ID: sg.ID, Kind: sg.Kind, Title: sg.Title, Detail: sg.Detail, Proposed: sg.Proposed,
		})
	}
	return out
}

func dedupeSuggestions(in []model.SEOSuggestion) []model.SEOSuggestion {
	seen := map[string]bool{}
	var out []model.SEOSuggestion
	for _, s := range in {
		key := s.Kind + "|" + s.Title + "|" + s.Proposed
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func parseOwnerRepo(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".git")
	if i := strings.Index(raw, "github.com/"); i >= 0 {
		path := strings.Trim(strings.SplitN(raw[i+11:], "?", 2)[0], "/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("repo must be owner/repo")
		}
		return parts[0], parts[1], nil
	}
	parts := strings.Split(raw, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repo must be owner/repo")
	}
	return parts[0], parts[1], nil
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
	if run.Progress != nil {
		run.Progress.Phase = "done"
	}
	_ = s.repo.Update(context.Background(), run)
}

func (s *seoService) cancel(run *model.SEORun) {
	now := time.Now()
	run.Status = "cancelled"
	msg := "cancelled"
	run.ErrorMessage = &msg
	run.CompletedAt = &now
	if run.Progress != nil {
		run.Progress.Phase = "done"
	}
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
