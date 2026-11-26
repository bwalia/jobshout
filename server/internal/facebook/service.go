package facebook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
)

// graphAPI is the slice of the Graph client the service uses.
type graphAPI interface {
	AuthURL(appID, redirectURL, state string) string
	Exchange(ctx context.Context, appID, appSecret, redirectURL, code string) (Token, error)
	Me(ctx context.Context, token string) (User, error)
	Accounts(ctx context.Context, userToken string) ([]Page, error)
	PageInfo(ctx context.Context, pageID, pageToken string) (Page, error)
	UpdatePage(ctx context.Context, pageID, pageToken string, u PageUpdate) error
	CreatePost(ctx context.Context, pageID, pageToken string, p Publish) (string, error)
	EditPost(ctx context.Context, postID, pageToken, message string) error
	DeletePost(ctx context.Context, postID, pageToken string) error
	PostStats(ctx context.Context, postID, pageToken string) (PostStats, error)
	Comments(ctx context.Context, postID, pageToken string, limit int) ([]Comment, error)
	ReplyToComment(ctx context.Context, commentID, pageToken, message string) (string, error)
}

type calendarDrafter interface {
	Calendar(ctx context.Context, req DraftRequest) ([]Draft, error)
}

// ImageMaker is the host's image generation. Nil or disabled means drafts
// carry an image idea but no picture.
type ImageMaker interface {
	Enabled() bool
	// Generate returns the stored image's URL.
	Generate(ctx context.Context, orgID, userID uuid.UUID, prompt string) (string, error)
}

// BuiltinAgents finds an org's seeded agent.
type BuiltinAgents interface {
	FindBuiltin(ctx context.Context, orgID uuid.UUID, builtin string) (*model.Agent, error)
}

// Tasks is the slice of the board the service writes results to.
type Tasks interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.Task, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateTaskRequest) (*model.Task, error)
	Transition(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error
}

// Service is the Facebook Page Manager: drafting, the connection, publishing,
// engagement and analytics.
type Service struct {
	store   *Store
	agents  BuiltinAgents
	api     graphAPI
	drafter calendarDrafter
	images  ImageMaker
	cfg     Config
	logger  *zap.Logger
	tasks   Tasks

	// ticking serialises the scheduler so one tick cannot overlap the next.
	ticking sync.Mutex
}

// NewService wires the Facebook Page Manager.
func NewService(store *Store, agents BuiltinAgents, api graphAPI, drafter calendarDrafter, images ImageMaker, cfg Config, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	// A nil *Drafter in the interface would read as configured.
	if d, ok := drafter.(*Drafter); ok && d == nil {
		drafter = nil
	}
	return &Service{store: store, agents: agents, api: api, drafter: drafter, images: images, cfg: cfg, logger: logger}
}

// BindTasks lets the service report back to the launching board task.
func (s *Service) BindTasks(t Tasks) { s.tasks = t }

// ── Launch: draft a calendar ────────────────────────────────────────────────

// LaunchRequest is a launch's values, resolved.
type LaunchRequest struct {
	OrgID     uuid.UUID
	UserID    uuid.UUID
	AgentID   uuid.UUID
	TaskID    *uuid.UUID
	Page      string // Page ID or name words; empty = the org's only Page
	Objective string
	Days      int
	PerWeek   int
	Notes     string
	Images    bool
}

// LaunchResult is what a launch started.
type LaunchResult struct {
	PageName string
	Posts    int
	Days     int
}

// Launch claims the calendar's rows and drafts them in the background.
func (s *Service) Launch(ctx context.Context, req LaunchRequest) (*LaunchResult, error) {
	if s.drafter == nil {
		return nil, errors.New("the Facebook Page Manager has no LLM configured")
	}
	page, err := s.resolvePage(ctx, req.OrgID, req.Page)
	if err != nil {
		return nil, err
	}
	days := clamp(req.Days, 1, 30, 7)
	perWeek := clamp(req.PerWeek, 1, 7, 3)
	count := clamp(days*perWeek/7, 1, 30, 3)
	rows, err := s.store.InsertDrafting(ctx, req.OrgID, page.PageID, req.TaskID, req.Notes, count)
	if err != nil {
		return nil, err
	}
	go s.draftAll(*page, rows, req, days)
	return &LaunchResult{PageName: page.Name, Posts: count, Days: days}, nil
}

func clamp(v, lo, hi, def int) int {
	if v <= 0 {
		return def
	}
	return min(max(v, lo), hi)
}

func (s *Service) resolvePage(ctx context.Context, orgID uuid.UUID, ref string) (*PageRecord, error) {
	pages, err := s.store.ListPages(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, ErrNotConnected
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		if len(pages) == 1 {
			return &pages[0], nil
		}
		names := make([]string, 0, len(pages))
		for _, p := range pages {
			names = append(names, p.Name)
		}
		return nil, fmt.Errorf("this org has several Facebook Pages (%s); say which one", strings.Join(names, ", "))
	}
	low := strings.ToLower(ref)
	for i := range pages {
		if pages[i].PageID == ref || strings.Contains(strings.ToLower(pages[i].Name), low) {
			return &pages[i], nil
		}
	}
	return nil, fmt.Errorf("no connected Facebook Page matches %q", ref)
}

func (s *Service) draftAll(page PageRecord, rows []Post, req LaunchRequest, days int) {
	timeout := s.cfg.DraftTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	trace := llmtrace.TraceInfo{
		TraceName: "go-facebook-calendar",
		SessionID: page.PageID,
		AgentID:   req.AgentID.String(),
		OrgID:     req.OrgID.String(),
	}
	if req.TaskID != nil {
		trace.TaskID = req.TaskID.String()
	}
	ctx = llmtrace.WithTrace(ctx, trace)
	log := s.logger.With(zap.String("page_id", page.PageID))

	dr := DraftRequest{Page: page, Objective: req.Objective, Count: len(rows), Days: days, Notes: req.Notes, Now: time.Now()}
	if recent, err := s.store.Recent(ctx, req.OrgID, page.PageID, 12); err == nil {
		for _, p := range recent {
			dr.Recent = append(dr.Recent, p.Message)
		}
	}
	if a, err := s.analytics(ctx, req.OrgID, page, false); err == nil {
		dr.Performance = performanceHint(a)
	}

	drafts, err := s.drafter.Calendar(ctx, dr)
	done, failed := 0, 0
	for i, row := range rows {
		switch {
		case err != nil:
			_ = s.store.FillDraft(context.Background(), row.ID, Draft{}, err.Error())
			failed++
		case i >= len(drafts):
			_ = s.store.FillDraft(context.Background(), row.ID, Draft{}, "the model wrote fewer posts than asked; redraft this one")
			failed++
		default:
			if ferr := s.store.FillDraft(context.Background(), row.ID, drafts[i], ""); ferr != nil {
				log.Error("facebook: store draft", zap.Error(ferr))
				failed++
				continue
			}
			done++
			if req.Images {
				s.makeImage(ctx, req.OrgID, req.UserID, row.ID, page, drafts[i].ImagePrompt)
			}
		}
	}
	if err != nil {
		log.Warn("facebook: calendar failed", zap.Error(err))
	}
	log.Info("facebook: drafting done", zap.Int("drafted", done), zap.Int("failed", failed))

	note := fmt.Sprintf("Facebook calendar for %q: %d post(s) drafted. Review, approve and schedule them on the Facebook Page Manager tab.", page.Name, done)
	status := "done"
	if done == 0 {
		note = fmt.Sprintf("Facebook drafting failed for %q; redraft on the Facebook Page Manager tab.", page.Name)
		status = ""
	} else if failed > 0 {
		note += fmt.Sprintf(" %d failed.", failed)
	}
	s.notifyBoard(req.TaskID, note, status)
}

func (s *Service) makeImage(ctx context.Context, orgID, userID, postID uuid.UUID, page PageRecord, idea string) {
	if s.images == nil || !s.images.Enabled() || strings.TrimSpace(idea) == "" {
		return
	}
	url, err := s.images.Generate(ctx, orgID, userID, ImagePrompt(idea, page))
	if err != nil || url == "" {
		s.logger.Warn("facebook: creative failed", zap.String("post_id", postID.String()), zap.Error(err))
		return
	}
	if err := s.store.SetImage(context.Background(), orgID, postID, url); err != nil {
		s.logger.Warn("facebook: store creative", zap.Error(err))
	}
}

func (s *Service) notifyBoard(taskID *uuid.UUID, note, status string) {
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

// ── Posts ───────────────────────────────────────────────────────────────────

// ListPosts lists an org's posts, optionally for one Page.
func (s *Service) ListPosts(ctx context.Context, orgID uuid.UUID, pageID string) ([]Post, error) {
	return s.store.ListPosts(ctx, orgID, pageID, 200)
}

// PostEdit is a change to a post that has not been published.
type PostEdit struct {
	Message string
	// ScheduledFor sets the time; ClearSchedule removes it.
	ScheduledFor  *time.Time
	ClearSchedule bool
}

// UpdatePost edits a draft. A scheduled post returns to draft and must be
// approved again.
func (s *Service) UpdatePost(ctx context.Context, orgID, id uuid.UUID, e PostEdit) (*Post, error) {
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		return nil, errors.New("the post text is empty")
	}
	if n := len([]rune(msg)); n > MessageMax {
		return nil, fmt.Errorf("the post is too long (%d of %d characters)", n, MessageMax)
	}
	return s.store.UpdateDraft(ctx, orgID, id, msg, e.ScheduledFor, e.ClearSchedule)
}

// Approve schedules a draft. A time at or before now publishes it immediately.
func (s *Service) Approve(ctx context.Context, orgID, userID, id uuid.UUID, when *time.Time) (*Post, error) {
	cur, err := s.store.GetPost(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	at := when
	if at == nil {
		at = cur.ScheduledFor
	}
	if at == nil {
		return nil, errors.New("choose a time to publish, or publish it now")
	}
	if _, _, err := s.page(ctx, orgID, cur.PageID); err != nil {
		return nil, err
	}
	if !at.After(time.Now().Add(time.Minute)) {
		return s.PublishNow(ctx, orgID, userID, id)
	}
	uid := userID
	return s.store.Schedule(ctx, orgID, id, at.UTC(), &uid)
}

// Unschedule returns a scheduled post to draft.
func (s *Service) Unschedule(ctx context.Context, orgID, id uuid.UUID) (*Post, error) {
	return s.store.Unschedule(ctx, orgID, id)
}

// PublishNow publishes a post immediately.
func (s *Service) PublishNow(ctx context.Context, orgID, userID, id uuid.UUID) (*Post, error) {
	cur, err := s.store.GetPost(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.page(ctx, orgID, cur.PageID); err != nil {
		return nil, err
	}
	uid := userID
	p, err := s.store.ClaimNow(ctx, orgID, id, &uid)
	if err != nil {
		return nil, err
	}
	return s.publish(*p)
}

// publish sends a claimed (publishing) post to Facebook.
func (s *Service) publish(p Post) (*Post, error) {
	// The call runs on its own deadline: a client that disconnects must not
	// leave the post stuck in publishing.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	release := func(err error) (*Post, error) {
		msg := err.Error()
		if errors.Is(err, ErrNoAnswer) {
			msg += " — check the Page before posting again; Facebook may have published it"
		}
		if rerr := s.store.ReleasePublishing(context.Background(), p.ID, msg); rerr != nil {
			s.logger.Error("facebook: release publishing", zap.String("post_id", p.ID.String()), zap.Error(rerr))
		}
		return nil, err
	}
	rec, token, err := s.page(ctx, p.OrgID, p.PageID)
	if err != nil {
		return release(err)
	}
	fbID, err := s.api.CreatePost(ctx, rec.PageID, token, Publish{Message: p.Message, ImageURL: s.absolute(p.ImageURL)})
	if err != nil {
		s.noteAuth(p.OrgID, p.PageID, err)
		return release(err)
	}
	posted, err := s.store.MarkPublished(context.Background(), p.ID, fbID, PostURL(fbID))
	if err != nil {
		// Facebook has it; only our record failed. Say so rather than invite a retry.
		s.logger.Error("facebook: published but not recorded", zap.String("post_id", p.ID.String()), zap.String("fb_post_id", fbID), zap.Error(err))
		return nil, fmt.Errorf("published to Facebook (%s) but could not record it: %w", fbID, err)
	}
	s.logger.Info("facebook: published", zap.String("post_id", p.ID.String()), zap.String("page_id", p.PageID))
	return posted, nil
}

// absolute makes a stored image URL fetchable by Facebook. Anything that is
// not an http(s) URL is dropped, so a post goes out as text rather than fail.
func (s *Service) absolute(u string) string {
	u = strings.TrimSpace(u)
	switch {
	case strings.HasPrefix(u, "https://"), strings.HasPrefix(u, "http://"):
		return u
	case strings.HasPrefix(u, "/"):
		return strings.TrimRight(s.cfg.FrontendBaseURL, "/") + u
	}
	return ""
}

// Delete removes a post. A published one is deleted on Facebook first.
func (s *Service) Delete(ctx context.Context, orgID, id uuid.UUID) error {
	p, err := s.store.GetPost(ctx, orgID, id)
	if err != nil {
		return err
	}
	if p.Status == StatusPublished && p.FBPostID != "" {
		_, token, err := s.page(ctx, orgID, p.PageID)
		if err != nil {
			return err
		}
		if err := s.api.DeletePost(ctx, p.FBPostID, token); err != nil {
			s.noteAuth(orgID, p.PageID, err)
			return err
		}
	}
	return s.store.DeletePost(ctx, orgID, id)
}

// EditPublished changes the text of a post already on Facebook.
func (s *Service) EditPublished(ctx context.Context, orgID, id uuid.UUID, message string) (*Post, error) {
	message = FitMessage(message)
	if message == "" {
		return nil, errors.New("the post text is empty")
	}
	p, err := s.store.GetPost(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if p.Status != StatusPublished || p.FBPostID == "" {
		return nil, ErrConflict
	}
	_, token, err := s.page(ctx, orgID, p.PageID)
	if err != nil {
		return nil, err
	}
	if err := s.api.EditPost(ctx, p.FBPostID, token, message); err != nil {
		s.noteAuth(orgID, p.PageID, err)
		return nil, err
	}
	// The row is the record of what is live: store the new text.
	_, err = s.store.pool.Exec(ctx, `UPDATE facebook_posts SET message=$3, updated_at=NOW() WHERE org_id=$1 AND id=$2`, orgID, id, message)
	if err != nil {
		return nil, err
	}
	return s.store.GetPost(ctx, orgID, id)
}

// Redraft writes one post again from the Page's profile.
func (s *Service) Redraft(ctx context.Context, orgID, userID, id uuid.UUID) (*Post, error) {
	if s.drafter == nil {
		return nil, errors.New("the Facebook Page Manager has no LLM configured")
	}
	cur, err := s.store.GetPost(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	page, _, err := s.store.GetPage(ctx, orgID, cur.PageID)
	if err != nil {
		return nil, err
	}
	row, err := s.store.ResetForRedraft(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	var agentID uuid.UUID
	if a, aerr := s.agents.FindBuiltin(ctx, orgID, Builtin); aerr == nil && a != nil {
		agentID = a.ID
	}
	go s.draftAll(*page, []Post{*row}, LaunchRequest{OrgID: orgID, UserID: userID, AgentID: agentID, Notes: cur.Notes, Objective: cur.Objective}, 7)
	return row, nil
}

// GenerateImage makes (or remakes) a post's creative from its image idea.
func (s *Service) GenerateImage(ctx context.Context, orgID, userID, id uuid.UUID) (*Post, error) {
	if s.images == nil || !s.images.Enabled() {
		return nil, errors.New("image generation is not configured on this deployment")
	}
	p, err := s.store.GetPost(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if p.ImagePrompt == "" {
		return nil, errors.New("this post has no image idea; edit the text and redraft it")
	}
	page, _, err := s.store.GetPage(ctx, orgID, p.PageID)
	if err != nil {
		return nil, err
	}
	url, err := s.images.Generate(ctx, orgID, userID, ImagePrompt(p.ImagePrompt, *page))
	if err != nil {
		return nil, err
	}
	if err := s.store.SetImage(ctx, orgID, id, url); err != nil {
		return nil, err
	}
	return s.store.GetPost(ctx, orgID, id)
}

// ── Scheduler ───────────────────────────────────────────────────────────────

// StartScheduler publishes due posts and sweeps abandoned rows until ctx ends.
func (s *Service) StartScheduler(ctx context.Context) {
	interval := s.cfg.PollInterval
	if interval <= 0 {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) tick(ctx context.Context) {
	if !s.ticking.TryLock() {
		return
	}
	defer s.ticking.Unlock()
	if n, err := s.store.FailStaleDrafting(ctx, time.Now().Add(-(s.cfg.DraftTimeout + 2*time.Minute))); err != nil {
		s.logger.Warn("facebook: sweep stale drafts", zap.Error(err))
	} else if n > 0 {
		s.logger.Info("facebook: failed interrupted drafts", zap.Int("count", n))
	}
	if n, err := s.store.FailStalePublishing(ctx, time.Now().Add(-5*time.Minute)); err != nil {
		s.logger.Warn("facebook: sweep stale publishing", zap.Error(err))
	} else if n > 0 {
		s.logger.Warn("facebook: released interrupted publishes", zap.Int("count", n))
	}
	if !s.cfg.Configured() {
		return
	}
	due, err := s.store.ClaimDue(ctx, time.Now(), 20)
	if err != nil {
		s.logger.Warn("facebook: claim due posts", zap.Error(err))
		return
	}
	for _, p := range due {
		if _, err := s.publish(p); err != nil {
			s.logger.Warn("facebook: scheduled publish failed", zap.String("post_id", p.ID.String()), zap.Error(err))
		}
	}
}

// ── Pages, connection ───────────────────────────────────────────────────────

func (s *Service) key() ([]byte, error) {
	if !s.cfg.Configured() {
		return nil, ErrNotConfigured
	}
	return mail.KeyFromSecret(s.cfg.TokenKey)
}

// page returns a Page and its decrypted token.
func (s *Service) page(ctx context.Context, orgID uuid.UUID, pageID string) (*PageRecord, string, error) {
	key, err := s.key()
	if err != nil {
		return nil, "", err
	}
	rec, enc, err := s.store.GetPage(ctx, orgID, pageID)
	if errors.Is(err, ErrNotFound) {
		return nil, "", ErrNotConnected
	}
	if err != nil {
		return nil, "", err
	}
	if rec.NeedsReauth {
		return nil, "", ErrTokenExpired
	}
	plain, err := mail.Decrypt(key, enc)
	if err != nil {
		return nil, "", fmt.Errorf("facebook: stored token unreadable (was FACEBOOK_TOKEN_KEY changed?) — connect Facebook again: %w", err)
	}
	return rec, string(plain), nil
}

// noteAuth flags a Page whose token Meta rejected, so the tab asks for a reconnect.
func (s *Service) noteAuth(orgID uuid.UUID, pageID string, err error) {
	if errors.Is(err, ErrTokenExpired) {
		s.store.MarkNeedsReauth(context.Background(), orgID, pageID)
	}
}

// Status is what the tab shows about the connection and the org's Pages.
func (s *Service) Status(ctx context.Context, orgID uuid.UUID) (*Status, error) {
	st := &Status{Configured: s.cfg.Configured(), Pages: []PageRecord{}, PageCreation: CreationGuide}
	pages, err := s.store.ListPages(ctx, orgID)
	if err != nil {
		return nil, err
	}
	st.Pages = pages
	conn, err := s.store.GetConnection(ctx, orgID)
	if errors.Is(err, ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.Connected = len(pages) > 0
	st.Name = conn.Name
	st.ExpiresAt = conn.ExpiresAt
	return st, nil
}

// StartConnect returns Facebook's consent URL.
func (s *Service) StartConnect(ctx context.Context, orgID, userID uuid.UUID) (string, error) {
	if !s.cfg.Configured() {
		return "", ErrNotConfigured
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("facebook: state: %w", err)
	}
	state := hex.EncodeToString(buf)
	if err := s.store.PutOAuthState(ctx, state, orgID, userID, time.Now().Add(10*time.Minute)); err != nil {
		return "", err
	}
	return s.api.AuthURL(s.cfg.AppID, s.cfg.RedirectURL, state), nil
}

// CompleteConnect finishes the consent redirect: it lists the user's Pages and
// stores each Page's sealed token. The user token is not kept.
func (s *Service) CompleteConnect(ctx context.Context, state, code string) error {
	key, err := s.key()
	if err != nil {
		return err
	}
	orgID, userID, err := s.store.ConsumeOAuthState(ctx, state)
	if err != nil {
		return errors.New("the Facebook sign-in expired or was already used; connect again")
	}
	tok, err := s.api.Exchange(ctx, s.cfg.AppID, s.cfg.AppSecret, s.cfg.RedirectURL, code)
	if err != nil {
		return err
	}
	me, err := s.api.Me(ctx, tok.AccessToken)
	if err != nil {
		return err
	}
	pages, err := s.api.Accounts(ctx, tok.AccessToken)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return errors.New("no Facebook Pages were shared with the app; connect again and choose the Pages to manage")
	}
	for _, p := range pages {
		if p.AccessToken == "" {
			continue
		}
		enc, err := mail.Encrypt(key, []byte(p.AccessToken))
		if err != nil {
			return err
		}
		if err := s.store.UpsertPage(ctx, orgID, p, enc); err != nil {
			return err
		}
	}
	uid := userID
	var exp *time.Time
	if !tok.Expiry.IsZero() {
		exp = &tok.Expiry
	}
	return s.store.UpsertConnection(ctx, Connection{OrgID: orgID, FBUserID: me.ID, Name: me.Name, ConnectedBy: &uid, ExpiresAt: exp})
}

// Disconnect forgets the connection and every Page token. Drafts are kept.
func (s *Service) Disconnect(ctx context.Context, orgID uuid.UUID) error {
	return s.store.Disconnect(ctx, orgID)
}

// SetProfile saves a Page's brand brief, voice and timezone.
func (s *Service) SetProfile(ctx context.Context, orgID uuid.UUID, pageID, brief, voice, tz string) (*PageRecord, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return nil, fmt.Errorf("unknown timezone %q (use a name like Europe/London)", tz)
	}
	return s.store.SetProfile(ctx, orgID, pageID, strings.TrimSpace(brief), strings.TrimSpace(voice), tz)
}

// SetPaused pauses or resumes automatic publishing for a Page. Scheduled posts
// stay scheduled while paused and publish (if still due) once resumed.
func (s *Service) SetPaused(ctx context.Context, orgID uuid.UUID, pageID string, paused bool) (*PageRecord, error) {
	return s.store.SetPaused(ctx, orgID, pageID, paused)
}

// RefreshPage re-reads a Page's details from Facebook.
func (s *Service) RefreshPage(ctx context.Context, orgID uuid.UUID, pageID string) (*PageRecord, error) {
	rec, token, err := s.page(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	info, err := s.api.PageInfo(ctx, rec.PageID, token)
	if err != nil {
		s.noteAuth(orgID, pageID, err)
		return nil, err
	}
	if err := s.store.SetPageInfo(ctx, orgID, info); err != nil {
		return nil, err
	}
	r, _, err := s.store.GetPage(ctx, orgID, pageID)
	return r, err
}

// UpdatePageInfo changes the Page's bio, description or website on Facebook.
func (s *Service) UpdatePageInfo(ctx context.Context, orgID uuid.UUID, pageID string, u PageUpdate) (*PageRecord, error) {
	rec, token, err := s.page(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	if err := s.api.UpdatePage(ctx, rec.PageID, token, u); err != nil {
		s.noteAuth(orgID, pageID, err)
		return nil, err
	}
	return s.RefreshPage(ctx, orgID, pageID)
}

// ── Engagement and analytics ────────────────────────────────────────────────

// Comments lists the newest comments on a published post.
func (s *Service) Comments(ctx context.Context, orgID, id uuid.UUID) ([]Comment, error) {
	p, token, err := s.publishedPost(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	out, err := s.api.Comments(ctx, p.FBPostID, token, 25)
	s.noteAuth(orgID, p.PageID, err)
	return out, err
}

// Reply posts a Page reply under a comment on one of the org's posts.
func (s *Service) Reply(ctx context.Context, orgID, id uuid.UUID, commentID, message string) error {
	message = strings.TrimSpace(message)
	if message == "" || commentID == "" {
		return errors.New("a comment and a reply are required")
	}
	p, token, err := s.publishedPost(ctx, orgID, id)
	if err != nil {
		return err
	}
	// A comment ID is <postID>_<commentID>; refuse one from another post so
	// a reply can only land under a post this org published.
	if !strings.HasPrefix(commentID, p.FBPostID+"_") {
		return errors.New("that comment is not on this post")
	}
	_, err = s.api.ReplyToComment(ctx, commentID, token, message)
	s.noteAuth(orgID, p.PageID, err)
	return err
}

func (s *Service) publishedPost(ctx context.Context, orgID, id uuid.UUID) (*Post, string, error) {
	p, err := s.store.GetPost(ctx, orgID, id)
	if err != nil {
		return nil, "", err
	}
	if p.Status != StatusPublished || p.FBPostID == "" {
		return nil, "", ErrConflict
	}
	_, token, err := s.page(ctx, orgID, p.PageID)
	if err != nil {
		return nil, "", err
	}
	return p, token, nil
}

// Analytics summarises a Page's recent published posts, refreshing counts
// from Facebook first.
func (s *Service) Analytics(ctx context.Context, orgID uuid.UUID, pageID string) (*Analytics, error) {
	rec, _, err := s.store.GetPage(ctx, orgID, pageID)
	if err != nil {
		return nil, err
	}
	return s.analytics(ctx, orgID, *rec, true)
}

func (s *Service) analytics(ctx context.Context, orgID uuid.UUID, page PageRecord, refresh bool) (*Analytics, error) {
	posts, err := s.store.Published(ctx, orgID, page.PageID, 30)
	if err != nil {
		return nil, err
	}
	if refresh && s.cfg.Configured() && len(posts) > 0 {
		if _, token, err := s.page(ctx, orgID, page.PageID); err == nil {
			for i := range posts {
				if posts[i].StatsAt != nil && time.Since(*posts[i].StatsAt) < 10*time.Minute {
					continue
				}
				st, err := s.api.PostStats(ctx, posts[i].FBPostID, token)
				if err != nil {
					s.noteAuth(orgID, page.PageID, err)
					s.logger.Warn("facebook: post stats", zap.String("post_id", posts[i].ID.String()), zap.Error(err))
					if errors.Is(err, ErrTokenExpired) {
						break
					}
					continue
				}
				if err := s.store.SetStats(ctx, posts[i].ID, PostStatsRow{Reactions: st.Reactions, Comments: st.Comments, Shares: st.Shares}); err == nil {
					posts[i].Reactions, posts[i].Comments, posts[i].Shares = st.Reactions, st.Comments, st.Shares
				}
			}
		}
	}
	return Summarise(page, posts), nil
}

// Summarise totals a Page's published posts and ranks its categories.
func Summarise(page PageRecord, posts []Post) *Analytics {
	a := &Analytics{PageID: page.PageID, Posts: len(posts), Followers: page.Followers, ByCategory: []CategoryResult{}}
	type agg struct{ n, eng int }
	cats := map[string]*agg{}
	for i, p := range posts {
		a.Reactions += p.Reactions
		a.Comments += p.Comments
		a.Shares += p.Shares
		c := p.Category
		if c == "" {
			c = "uncategorised"
		}
		if cats[c] == nil {
			cats[c] = &agg{}
		}
		cats[c].n++
		cats[c].eng += p.Engagement()
		if a.Top == nil || p.Engagement() > a.Top.Engagement() {
			top := posts[i]
			a.Top = &top
		}
	}
	if a.Posts > 0 {
		a.AvgPerPost = float64(a.Reactions+a.Comments+a.Shares) / float64(a.Posts)
	}
	for c, g := range cats {
		a.ByCategory = append(a.ByCategory, CategoryResult{Category: c, Posts: g.n, AvgPerPost: float64(g.eng) / float64(g.n)})
	}
	sort.Slice(a.ByCategory, func(i, j int) bool {
		if a.ByCategory[i].AvgPerPost != a.ByCategory[j].AvgPerPost {
			return a.ByCategory[i].AvgPerPost > a.ByCategory[j].AvgPerPost
		}
		return a.ByCategory[i].Category < a.ByCategory[j].Category
	})
	return a
}

// performanceHint turns analytics into one plain sentence for the drafter, or
// "" when there is too little data to say anything honest.
func performanceHint(a *Analytics) string {
	if a == nil || a.Posts < 3 {
		return ""
	}
	var best, worst *CategoryResult
	for i := range a.ByCategory {
		c := &a.ByCategory[i]
		if c.Posts < 2 {
			continue
		}
		if best == nil {
			best = c
		}
		worst = c
	}
	if best == nil || worst == nil || best == worst {
		return ""
	}
	return fmt.Sprintf("%s posts averaged %.1f engagements, %s posts %.1f; lean towards what worked without dropping variety.",
		best.Category, best.AvgPerPost, worst.Category, worst.AvgPerPost)
}
