package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/linkedin"
	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

var (
	// ErrLinkedInNotConfigured: the deployment has no LinkedIn app.
	ErrLinkedInNotConfigured = errors.New("LinkedIn is not configured on this deployment (LINKEDIN_CLIENT_ID, LINKEDIN_CLIENT_SECRET, LINKEDIN_TOKEN_KEY)")
	// ErrLinkedInNotConnected: nobody in the org has connected LinkedIn.
	ErrLinkedInNotConnected = errors.New("connect LinkedIn on the LinkedIn Poster tab before posting")
	// ErrLinkedInNoArticle: nothing published matches the launch.
	ErrLinkedInNoArticle = errors.New("no published article found; publish an article first, or give its ID or title words")
	// ErrLinkedInNothingToDraft: every requested style is already posted or in flight.
	ErrLinkedInNothingToDraft = errors.New("this article's LinkedIn posts are already posted or being worked on; edit or redraft them on the LinkedIn Poster tab")
)

// LinkedInService is the LinkedIn Poster: drafting, the LinkedIn connection,
// and posting approved drafts.
type LinkedInService interface {
	linkedin.Runner
	Status(ctx context.Context, orgID uuid.UUID) (*model.LinkedInStatus, error)
	StartConnect(ctx context.Context, orgID, userID uuid.UUID) (string, error)
	CompleteConnect(ctx context.Context, state, code string) error
	Disconnect(ctx context.Context, orgID uuid.UUID) error
	ListPosts(ctx context.Context, orgID uuid.UUID) ([]model.LinkedInPost, error)
	UpdatePost(ctx context.Context, orgID, id uuid.UUID, commentary string) (*model.LinkedInPost, error)
	Redraft(ctx context.Context, orgID, id uuid.UUID) (*model.LinkedInPost, error)
	Publish(ctx context.Context, orgID, userID, id uuid.UUID) (*model.LinkedInPost, error)
	// StartAutoDraft drafts posts for newly published articles, and fails
	// drafts whose drafter died, until ctx ends.
	StartAutoDraft(ctx context.Context)
	BindTasks(tasks TaskService)
}

// linkedInAPI is the slice of the LinkedIn client the service uses.
type linkedInAPI interface {
	Exchange(ctx context.Context, clientID, clientSecret, redirectURL, code string) (linkedin.Token, error)
	Me(ctx context.Context, accessToken string) (linkedin.Member, error)
	CreatePost(ctx context.Context, accessToken string, p linkedin.Post) (string, error)
}

type linkedInDrafter interface {
	Draft(ctx context.Context, req linkedin.DraftRequest) (string, error)
}

type builtinAgents interface {
	FindBuiltin(ctx context.Context, orgID uuid.UUID, builtin string) (*model.Agent, error)
}

type linkedInService struct {
	repo    repository.LinkedInRepository
	agents  builtinAgents
	api     linkedInAPI
	drafter linkedInDrafter
	cfg     linkedin.Config
	logger  *zap.Logger
	tasks   TaskService

	// drafting serialises auto-drafting so one tick cannot overlap the next.
	drafting sync.Mutex
}

// NewLinkedInService wires the LinkedIn Poster.
func NewLinkedInService(repo repository.LinkedInRepository, agents builtinAgents, api linkedInAPI,
	drafter linkedInDrafter, cfg linkedin.Config, logger *zap.Logger) LinkedInService {
	if logger == nil {
		logger = zap.NewNop()
	}
	// A nil *Drafter in the interface would read as configured.
	if d, ok := drafter.(*linkedin.Drafter); ok && d == nil {
		drafter = nil
	}
	return &linkedInService{repo: repo, agents: agents, api: api, drafter: drafter, cfg: cfg, logger: logger}
}

func (s *linkedInService) BindTasks(tasks TaskService) { s.tasks = tasks }

// ── Drafting ────────────────────────────────────────────────────────────────

func (s *linkedInService) Launch(ctx context.Context, req linkedin.LaunchRequest) (*linkedin.LaunchResult, error) {
	if s.drafter == nil {
		return nil, errors.New("the LinkedIn Poster has no LLM configured")
	}
	article, err := s.resolveArticle(ctx, req.OrgID, req.Article)
	if err != nil {
		return nil, err
	}
	variants := req.Variants
	if len(variants) == 0 {
		variants = model.LinkedInVariants
	}
	claimed, err := s.repo.ClaimDrafts(ctx, model.LinkedInPost{
		OrgID:        req.OrgID,
		ArticleID:    article.ID,
		ArticleTitle: article.Title,
		TaskID:       req.TaskID,
		LinkURL:      s.linkFor(article),
		Notes:        req.Notes,
	}, variants, true)
	if err != nil {
		return nil, err
	}
	if len(claimed) == 0 {
		return nil, ErrLinkedInNothingToDraft
	}
	agentID := req.AgentID
	go s.draftAll(*article, claimed, agentID, req.TaskID)

	got := make([]string, 0, len(claimed))
	for _, p := range claimed {
		got = append(got, p.Variant)
	}
	return &linkedin.LaunchResult{ArticleID: article.ID, ArticleTitle: article.Title, Variants: got}, nil
}

func (s *linkedInService) resolveArticle(ctx context.Context, orgID uuid.UUID, ref string) (*model.LinkedInArticle, error) {
	ref = strings.TrimSpace(ref)
	var (
		a   *model.LinkedInArticle
		err error
	)
	switch id, perr := uuid.Parse(ref); {
	case ref == "":
		a, err = s.repo.LatestUndrafted(ctx, orgID)
	case perr == nil:
		a, err = s.repo.FindArticle(ctx, orgID, id)
	default:
		a, err = s.repo.FindArticleByTitle(ctx, orgID, ref)
	}
	if errors.Is(err, repository.ErrLinkedInNotFound) {
		return nil, ErrLinkedInNoArticle
	}
	return a, err
}

// linkFor is the article's public URL, when it has one.
func (s *linkedInService) linkFor(a *model.LinkedInArticle) string {
	if a.InsightsSlug == "" || s.cfg.InsightsSiteURL == "" {
		return ""
	}
	return strings.TrimRight(s.cfg.InsightsSiteURL, "/") + "/insights/" + a.InsightsSlug
}

// draftAll writes each claimed row. It runs detached from the request: the
// rows are already claimed, and a row whose drafter dies is failed by the
// auto-draft loop's sweep.
func (s *linkedInService) draftAll(article model.LinkedInArticle, posts []model.LinkedInPost, agentID uuid.UUID, taskID *uuid.UUID) {
	timeout := s.cfg.DraftTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	trace := llmtrace.TraceInfo{
		TraceName: "go-linkedin-draft",
		SessionID: article.ID.String(),
		AgentID:   agentID.String(),
		OrgID:     article.OrgID.String(),
	}
	if taskID != nil {
		trace.TaskID = taskID.String()
	}
	ctx = llmtrace.WithTrace(ctx, trace)
	log := s.logger.With(zap.String("article_id", article.ID.String()))

	var done, failed []string
	for _, p := range posts {
		text, err := s.drafter.Draft(ctx, linkedin.DraftRequest{
			Article: article,
			Variant: p.Variant,
			Notes:   p.Notes,
			HasLink: p.LinkURL != "",
		})
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
			failed = append(failed, p.Variant)
			log.Warn("linkedin: draft failed", zap.String("variant", p.Variant), zap.Error(err))
		} else {
			done = append(done, p.Variant)
		}
		if ferr := s.repo.FinishDraft(context.Background(), p.ID, text, errMsg); ferr != nil {
			log.Error("linkedin: store draft", zap.String("variant", p.Variant), zap.Error(ferr))
		}
	}
	log.Info("linkedin: drafting done", zap.Strings("drafted", done), zap.Strings("failed", failed))

	note := fmt.Sprintf("LinkedIn drafts ready for %q (%s). Review and post them on the LinkedIn Poster tab.", article.Title, strings.Join(done, ", "))
	status := "done"
	if len(done) == 0 {
		note = fmt.Sprintf("LinkedIn drafting failed for %q; redraft it on the LinkedIn Poster tab.", article.Title)
		status = ""
	} else if len(failed) > 0 {
		note += fmt.Sprintf(" Failed: %s.", strings.Join(failed, ", "))
	}
	s.notifyBoard(taskID, note, status)
}

func (s *linkedInService) notifyBoard(taskID *uuid.UUID, note, status string) {
	if s.tasks == nil || taskID == nil {
		return
	}
	ctx := context.Background()
	n := note
	if task, err := s.tasks.GetByID(ctx, *taskID); err == nil && task != nil && task.Description != nil && *task.Description != "" {
		n = *task.Description + "\n\n" + note
	}
	_, _ = s.tasks.Update(ctx, *taskID, model.UpdateTaskRequest{Description: &n})
	if status != "" {
		_ = s.tasks.Transition(ctx, *taskID, status, nil)
	}
}

func (s *linkedInService) Redraft(ctx context.Context, orgID, id uuid.UUID) (*model.LinkedInPost, error) {
	if s.drafter == nil {
		return nil, errors.New("the LinkedIn Poster has no LLM configured")
	}
	p, err := s.repo.GetPost(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	article, err := s.repo.FindArticle(ctx, orgID, p.ArticleID)
	if err != nil {
		return nil, err
	}
	claimed, err := s.repo.ClaimDrafts(ctx, model.LinkedInPost{
		OrgID:        orgID,
		ArticleID:    article.ID,
		ArticleTitle: article.Title,
		TaskID:       p.TaskID,
		LinkURL:      s.linkFor(article),
		Notes:        p.Notes,
	}, []string{p.Variant}, true)
	if err != nil {
		return nil, err
	}
	if len(claimed) == 0 {
		return nil, repository.ErrLinkedInConflict
	}
	var agentID uuid.UUID
	if a, err := s.agents.FindBuiltin(ctx, orgID, model.BuiltinLinkedIn); err == nil && a != nil {
		agentID = a.ID
	}
	go s.draftAll(*article, claimed, agentID, nil)
	return &claimed[0], nil
}

// StartAutoDraft ticks until ctx ends.
func (s *linkedInService) StartAutoDraft(ctx context.Context) {
	interval := s.cfg.PollInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.autoDraftOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// autoDraftOnce fails abandoned drafting rows, then drafts for articles
// published since the lookback in every org that connected LinkedIn.
// Claiming is the unique (article, variant) insert, so two replicas ticking
// together draft each article once.
func (s *linkedInService) autoDraftOnce(ctx context.Context) {
	if !s.drafting.TryLock() {
		return
	}
	defer s.drafting.Unlock()

	stale := time.Now().Add(-(s.cfg.DraftTimeout + 2*time.Minute))
	if n, err := s.repo.FailStaleDrafting(ctx, stale); err != nil {
		s.logger.Warn("linkedin: sweep stale drafts", zap.Error(err))
	} else if n > 0 {
		s.logger.Info("linkedin: failed interrupted drafts", zap.Int("count", n))
	}
	if !s.cfg.AutoDraft || s.drafter == nil {
		return
	}
	orgs, err := s.repo.ConnectedOrgs(ctx)
	if err != nil {
		s.logger.Warn("linkedin: list connected orgs", zap.Error(err))
		return
	}
	since := time.Now().Add(-s.cfg.Lookback)
	for _, orgID := range orgs {
		if ctx.Err() != nil {
			return
		}
		agent, err := s.agents.FindBuiltin(ctx, orgID, model.BuiltinLinkedIn)
		if err != nil || agent == nil {
			continue // the org removed the agent: it does not want drafts
		}
		articles, err := s.repo.PublishedSince(ctx, orgID, since, 5)
		if err != nil {
			s.logger.Warn("linkedin: list new articles", zap.String("org_id", orgID.String()), zap.Error(err))
			continue
		}
		for i := range articles {
			a := articles[i]
			claimed, err := s.repo.ClaimDrafts(ctx, model.LinkedInPost{
				OrgID: orgID, ArticleID: a.ID, ArticleTitle: a.Title, LinkURL: s.linkFor(&a),
			}, model.LinkedInVariants, false)
			if err != nil {
				s.logger.Warn("linkedin: claim drafts", zap.String("article_id", a.ID.String()), zap.Error(err))
				continue
			}
			if len(claimed) > 0 {
				s.draftAll(a, claimed, agent.ID, nil)
			}
		}
	}
}

// ── Posting ─────────────────────────────────────────────────────────────────

func (s *linkedInService) ListPosts(ctx context.Context, orgID uuid.UUID) ([]model.LinkedInPost, error) {
	return s.repo.ListPosts(ctx, orgID, 100)
}

func (s *linkedInService) UpdatePost(ctx context.Context, orgID, id uuid.UUID, commentary string) (*model.LinkedInPost, error) {
	commentary = strings.TrimSpace(commentary)
	if commentary == "" {
		return nil, errors.New("the post text is empty")
	}
	if n := len([]rune(linkedin.EscapeCommentary(commentary))); n > model.LinkedInCommentaryMax {
		return nil, fmt.Errorf("the post is too long for LinkedIn (%d of %d characters)", n, model.LinkedInCommentaryMax)
	}
	return s.repo.UpdateCommentary(ctx, orgID, id, commentary)
}

func (s *linkedInService) Publish(ctx context.Context, orgID, userID, id uuid.UUID) (*model.LinkedInPost, error) {
	token, conn, err := s.accessToken(ctx, orgID)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.ClaimPosting(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	// The call runs on its own deadline: a client that disconnects must not
	// leave the post stuck in posting.
	callCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	urn, err := s.api.CreatePost(callCtx, token, linkedin.Post{
		AuthorURN:  conn.MemberURN,
		Commentary: p.Commentary,
		LinkURL:    p.LinkURL,
		LinkTitle:  p.ArticleTitle,
	})
	if err != nil {
		msg := err.Error()
		if errors.Is(err, linkedin.ErrNoAnswer) {
			// LinkedIn may have published it anyway.
			msg += " — check your LinkedIn feed before posting again"
		}
		if rerr := s.repo.ReleasePosting(context.Background(), p.ID, msg); rerr != nil {
			s.logger.Error("linkedin: release posting", zap.String("post_id", p.ID.String()), zap.Error(rerr))
		}
		return nil, err
	}
	uid := userID
	posted, err := s.repo.MarkPosted(context.Background(), p.ID, urn, linkedin.PostURL(urn), &uid)
	if err != nil {
		// LinkedIn has it; only our record failed. Say so rather than invite a retry.
		s.logger.Error("linkedin: posted but not recorded", zap.String("post_id", p.ID.String()), zap.String("urn", urn), zap.Error(err))
		return nil, fmt.Errorf("posted to LinkedIn (%s) but could not record it: %w", urn, err)
	}
	s.logger.Info("linkedin: posted", zap.String("post_id", p.ID.String()), zap.String("variant", p.Variant))
	return posted, nil
}

// ── Connection ──────────────────────────────────────────────────────────────

func (s *linkedInService) tokenKey() ([]byte, error) {
	if !s.cfg.Configured() {
		return nil, ErrLinkedInNotConfigured
	}
	return mail.KeyFromSecret(s.cfg.TokenKey)
}

func (s *linkedInService) accessToken(ctx context.Context, orgID uuid.UUID) (string, *model.LinkedInConnection, error) {
	key, err := s.tokenKey()
	if err != nil {
		return "", nil, err
	}
	conn, enc, err := s.repo.GetConnection(ctx, orgID)
	if errors.Is(err, repository.ErrLinkedInNotFound) {
		return "", nil, ErrLinkedInNotConnected
	}
	if err != nil {
		return "", nil, err
	}
	if time.Now().After(conn.ExpiresAt) {
		return "", nil, linkedin.ErrTokenExpired
	}
	plain, err := mail.Decrypt(key, enc)
	if err != nil {
		return "", nil, fmt.Errorf("linkedin: stored token unreadable (was LINKEDIN_TOKEN_KEY changed?) — connect LinkedIn again: %w", err)
	}
	return string(plain), conn, nil
}

func (s *linkedInService) Status(ctx context.Context, orgID uuid.UUID) (*model.LinkedInStatus, error) {
	st := &model.LinkedInStatus{Configured: s.cfg.Configured(), AutoDraft: s.cfg.AutoDraft}
	conn, _, err := s.repo.GetConnection(ctx, orgID)
	if errors.Is(err, repository.ErrLinkedInNotFound) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	exp := conn.ExpiresAt
	st.Connected = true
	st.Name = conn.Name
	st.ExpiresAt = &exp
	left := time.Until(exp)
	st.Expired = left <= 0
	if !st.Expired {
		st.DaysLeft = int(math.Ceil(left.Hours() / 24))
	}
	return st, nil
}

func (s *linkedInService) StartConnect(ctx context.Context, orgID, userID uuid.UUID) (string, error) {
	if !s.cfg.Configured() {
		return "", ErrLinkedInNotConfigured
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("linkedin: state: %w", err)
	}
	state := hex.EncodeToString(buf)
	if err := s.repo.PutOAuthState(ctx, state, orgID, userID, time.Now().Add(10*time.Minute)); err != nil {
		return "", err
	}
	return linkedin.AuthURL(s.cfg.ClientID, s.cfg.RedirectURL, state), nil
}

func (s *linkedInService) CompleteConnect(ctx context.Context, state, code string) error {
	key, err := s.tokenKey()
	if err != nil {
		return err
	}
	orgID, userID, err := s.repo.ConsumeOAuthState(ctx, state)
	if err != nil {
		return errors.New("the LinkedIn sign-in expired or was already used; connect again")
	}
	tok, err := s.api.Exchange(ctx, s.cfg.ClientID, s.cfg.ClientSecret, s.cfg.RedirectURL, code)
	if err != nil {
		return err
	}
	member, err := s.api.Me(ctx, tok.AccessToken)
	if err != nil {
		return err
	}
	enc, err := mail.Encrypt(key, []byte(tok.AccessToken))
	if err != nil {
		return err
	}
	uid := userID
	return s.repo.UpsertConnection(ctx, model.LinkedInConnection{
		OrgID:       orgID,
		MemberURN:   member.URN,
		Name:        member.Name,
		ConnectedBy: &uid,
		ExpiresAt:   tok.Expiry,
	}, enc)
}

func (s *linkedInService) Disconnect(ctx context.Context, orgID uuid.UUID) error {
	return s.repo.DeleteConnection(ctx, orgID)
}
