package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
)

type mailMemRepo struct {
	conns   map[uuid.UUID]*model.MailConnection
	states  map[string]oauthState
	threads map[uuid.UUID]*model.MailThread
	drafts  map[uuid.UUID]*model.MailDraft
}

type oauthState struct {
	orgID, userID uuid.UUID
	expires       time.Time
}

func newMailMemRepo() *mailMemRepo {
	return &mailMemRepo{
		conns:   map[uuid.UUID]*model.MailConnection{},
		states:  map[string]oauthState{},
		threads: map[uuid.UUID]*model.MailThread{},
		drafts:  map[uuid.UUID]*model.MailDraft{},
	}
}

func cloneConn(c *model.MailConnection) *model.MailConnection {
	cp := *c
	if c.RefreshTokenEnc != nil {
		cp.RefreshTokenEnc = append([]byte(nil), c.RefreshTokenEnc...)
	}
	return &cp
}

func (m *mailMemRepo) UpsertConnection(ctx context.Context, c *model.MailConnection) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	c.UpdatedAt = time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = c.UpdatedAt
	}
	m.conns[c.OrgID] = cloneConn(c)
	return nil
}

func (m *mailMemRepo) GetConnectionByOrg(ctx context.Context, orgID uuid.UUID) (*model.MailConnection, error) {
	c, ok := m.conns[orgID]
	if !ok {
		return nil, nil
	}
	return cloneConn(c), nil
}

func (m *mailMemRepo) Disconnect(ctx context.Context, orgID uuid.UUID) error {
	c, ok := m.conns[orgID]
	if !ok {
		return nil
	}
	c.RefreshTokenEnc = nil
	c.GoogleEmail = ""
	c.Status = model.MailConnDisconnected
	now := time.Now()
	c.DisconnectedAt = &now
	return nil
}

func (m *mailMemRepo) UpdateConnectionMeta(ctx context.Context, c *model.MailConnection) error {
	ex, ok := m.conns[c.OrgID]
	if !ok {
		return errors.New("no connection")
	}
	ex.AllowMailboxMutations = c.AllowMailboxMutations
	ex.WatchLabels = c.WatchLabels
	ex.WatchSenders = c.WatchSenders
	ex.WatchSubjectPrefixes = c.WatchSubjectPrefixes
	ex.WatchKnowledgeURLs = c.WatchKnowledgeURLs
	ex.KnowledgeNotes = c.KnowledgeNotes
	ex.ResearchFocus = c.ResearchFocus
	ex.ReplyInstructions = c.ReplyInstructions
	return nil
}

func (m *mailMemRepo) PutOAuthState(ctx context.Context, state string, orgID, userID uuid.UUID, expires time.Time) error {
	m.states[state] = oauthState{orgID, userID, expires}
	return nil
}

func (m *mailMemRepo) ConsumeOAuthState(ctx context.Context, state string) (uuid.UUID, uuid.UUID, error) {
	st, ok := m.states[state]
	if !ok {
		return uuid.Nil, uuid.Nil, errors.New("oauth state not found")
	}
	delete(m.states, state)
	if time.Now().After(st.expires) {
		return uuid.Nil, uuid.Nil, errors.New("oauth state expired")
	}
	return st.orgID, st.userID, nil
}

func (m *mailMemRepo) UpsertThread(ctx context.Context, t *model.MailThread) error {
	for _, ex := range m.threads {
		if ex.OrgID == t.OrgID && ex.GmailThreadID == t.GmailThreadID {
			t.ID = ex.ID
			t.Status = ex.Status
			t.CreatedAt = ex.CreatedAt
			ex.GmailMessageID = t.GmailMessageID
			ex.FromEmail, ex.FromName, ex.ToEmail = t.FromEmail, t.FromName, t.ToEmail
			ex.Subject, ex.Snippet, ex.BodyText = t.Subject, t.Snippet, t.BodyText
			ex.UpdatedAt = time.Now()
			return nil
		}
	}
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	cp := *t
	m.threads[t.ID] = &cp
	return nil
}

func (m *mailMemRepo) GetThread(ctx context.Context, id uuid.UUID) (*model.MailThread, error) {
	t, ok := m.threads[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (m *mailMemRepo) GetThreadByGmailID(ctx context.Context, orgID uuid.UUID, gmailThreadID string) (*model.MailThread, error) {
	for _, t := range m.threads {
		if t.OrgID == orgID && t.GmailThreadID == gmailThreadID {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mailMemRepo) ListThreads(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailThread], error) {
	var data []model.MailThread
	for _, t := range m.threads {
		if t.OrgID == orgID {
			data = append(data, *t)
		}
	}
	return &model.PaginatedResponse[model.MailThread]{Data: data, Total: len(data), Page: 1, PerPage: 20, TotalPages: 1}, nil
}

func (m *mailMemRepo) UpdateThread(ctx context.Context, t *model.MailThread) error {
	ex, ok := m.threads[t.ID]
	if !ok {
		return errors.New("thread not found")
	}
	*ex = *t
	ex.UpdatedAt = time.Now()
	return nil
}

func (m *mailMemRepo) UpsertDraft(ctx context.Context, d *model.MailDraft) error {
	for _, ex := range m.drafts {
		if ex.ThreadID == d.ThreadID {
			d.ID = ex.ID
			ex.Subject, ex.Body, ex.ToEmail, ex.CCEmail = d.Subject, d.Body, d.ToEmail, d.CCEmail
			ex.Status = d.Status
			ex.ResearchBriefID = d.ResearchBriefID
			ex.UpdatedAt = time.Now()
			return nil
		}
	}
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	d.CreatedAt = time.Now()
	d.UpdatedAt = d.CreatedAt
	cp := *d
	m.drafts[d.ID] = &cp
	return nil
}

func (m *mailMemRepo) GetDraft(ctx context.Context, id uuid.UUID) (*model.MailDraft, error) {
	d, ok := m.drafts[id]
	if !ok {
		return nil, nil
	}
	cp := *d
	return &cp, nil
}

func (m *mailMemRepo) GetDraftByThread(ctx context.Context, threadID uuid.UUID) (*model.MailDraft, error) {
	for _, d := range m.drafts {
		if d.ThreadID == threadID {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mailMemRepo) ListDraftsByStatus(ctx context.Context, orgID uuid.UUID, status string, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailDraft], error) {
	var data []model.MailDraft
	for _, d := range m.drafts {
		if d.OrgID == orgID && d.Status == status {
			data = append(data, *d)
		}
	}
	return &model.PaginatedResponse[model.MailDraft]{Data: data, Total: len(data), Page: 1, PerPage: 20, TotalPages: 1}, nil
}

func (m *mailMemRepo) UpdateDraft(ctx context.Context, d *model.MailDraft) error {
	ex, ok := m.drafts[d.ID]
	if !ok {
		return errors.New("draft not found")
	}
	*ex = *d
	ex.UpdatedAt = time.Now()
	return nil
}

func (m *mailMemRepo) ClaimDueConnections(ctx context.Context, limit int, lease time.Duration) ([]model.MailConnection, error) {
	now := time.Now()
	var out []model.MailConnection
	for _, c := range m.conns {
		if c.Status != model.MailConnConnected && c.Status != model.MailConnError {
			continue
		}
		if len(c.RefreshTokenEnc) == 0 {
			continue
		}
		if c.NextSyncAt != nil && c.NextSyncAt.After(now) {
			continue
		}
		until := now.Add(lease)
		c.SyncLeaseUntil = &until
		out = append(out, *cloneConn(c))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *mailMemRepo) MarkSynced(ctx context.Context, id uuid.UUID, lastSync, nextSync time.Time) error {
	for _, c := range m.conns {
		if c.ID == id {
			c.LastSyncAt = &lastSync
			c.NextSyncAt = &nextSync
			c.SyncLeaseUntil = nil
			c.Status = model.MailConnConnected
			c.StatusError = nil
		}
	}
	return nil
}

type mailTestAgents struct {
	byID      map[uuid.UUID]*model.Agent
	byBuiltin map[string]*model.Agent
}

func newMailTestAgents() *mailTestAgents {
	return &mailTestAgents{byID: map[uuid.UUID]*model.Agent{}, byBuiltin: map[string]*model.Agent{}}
}

func (a *mailTestAgents) Create(ctx context.Context, agent *model.Agent) error {
	cp := *agent
	a.byID[agent.ID] = &cp
	if agent.Metadata != nil {
		if b, ok := agent.Metadata[model.MetadataKeyBuiltin].(string); ok {
			key := agent.OrgID.String() + ":" + b
			a.byBuiltin[key] = &cp
		}
	}
	return nil
}

func (a *mailTestAgents) FindBuiltin(ctx context.Context, orgID uuid.UUID, builtin string) (*model.Agent, error) {
	ag, ok := a.byBuiltin[orgID.String()+":"+builtin]
	if !ok {
		return nil, nil
	}
	cp := *ag
	return &cp, nil
}

func (a *mailTestAgents) FindByID(ctx context.Context, id uuid.UUID) (*model.Agent, error) {
	return a.byID[id], nil
}
func (a *mailTestAgents) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams, filter repository.AgentListFilter) (*model.PaginatedResponse[model.Agent], error) {
	return nil, nil
}
func (a *mailTestAgents) Update(ctx context.Context, agent *model.Agent) error {
	return nil
}
func (a *mailTestAgents) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (a *mailTestAgents) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return nil
}

type fakeGmail struct {
	email     string
	messages  []mail.InboxMessage
	sent      []mail.OutboundMessage
	sendCalls int
	listCalls int
	listErr   error
	lastQuery string
	tokens    mail.TokenSet
}

func (f *fakeGmail) ExchangeCode(ctx context.Context, code, redirectURL, clientID, clientSecret string) (mail.TokenSet, error) {
	return f.tokens, nil
}
func (f *fakeGmail) Refresh(ctx context.Context, refreshToken, clientID, clientSecret string) (mail.TokenSet, error) {
	return f.tokens, nil
}
func (f *fakeGmail) Profile(ctx context.Context, accessToken string) (string, error) {
	return f.email, nil
}
func (f *fakeGmail) ListMessages(ctx context.Context, accessToken, query string, limit int) ([]mail.InboxMessage, error) {
	f.listCalls++
	f.lastQuery = query
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.messages, nil
}
func (f *fakeGmail) Send(ctx context.Context, accessToken string, msg mail.OutboundMessage) (string, error) {
	f.sendCalls++
	f.sent = append(f.sent, msg)
	return "gmail-sent-1", nil
}

type scriptClass struct {
	result mail.ClassifyResult
}

func (s scriptClass) Classify(ctx context.Context, msg mail.InboxMessage) (mail.ClassifyResult, error) {
	return s.result, nil
}

type fakeResearch struct {
	calls   int
	lastReq research.Request
	brief   *research.Brief
}

func (f *fakeResearch) Research(ctx context.Context, orgID uuid.UUID, req research.Request, progress research.ProgressFunc) (*research.Brief, error) {
	f.calls++
	f.lastReq = req
	if f.brief != nil {
		return f.brief, nil
	}
	return &research.Brief{Topic: req.Topic, Summary: "verified fact", Findings: []research.Finding{{
		Claim: "verified fact", SourceURL: "https://example.com", Quote: "fact",
	}}, Sources: []research.Source{{URL: "https://example.com", Title: "ex"}}}, nil
}
func (f *fakeResearch) Trending(ctx context.Context, limit int) ([]research.TrendingItem, error) {
	return nil, nil
}
func (f *fakeResearch) Discover(ctx context.Context, orgID uuid.UUID, req research.DiscoverRequest, progress research.ProgressFunc) ([]research.Topic, error) {
	return nil, nil
}
func (f *fakeResearch) EnsureResearcher(ctx context.Context, orgID uuid.UUID) (*model.Agent, error) {
	return &model.Agent{Name: "Research Agent"}, nil
}
func (f *fakeResearch) Available() bool { return true }

func mailTestCfg() mail.Config {
	return mail.Config{
		ClientID: "cid", ClientSecret: "csec",
		RedirectURL:     "http://localhost/api/v1/mail/connection/oauth/callback",
		TokenKey:        "mail-unit-test-key",
		FrontendBaseURL: "http://localhost:3001",
		PollInterval:    5 * time.Minute,
	}
}

func setupMail(t *testing.T, gmail *fakeGmail, class mail.Classifier, researchSvc ResearchService) (*mailService, *mailMemRepo, uuid.UUID) {
	t.Helper()
	repo := newMailMemRepo()
	agents := newMailTestAgents()
	cfg := mailTestCfg()
	if class == nil {
		class = mail.NewClassifier(nil, zap.NewNop())
	}
	drafter := mail.NewDrafter(nil, "", zap.NewNop())
	svc := NewMailService(repo, agents, gmail, class, drafter, researchSvc, cfg, zap.NewNop()).(*mailService)
	orgID := uuid.New()
	if _, err := svc.EnsureMailAgent(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	return svc, repo, orgID
}

func connectOrg(t *testing.T, svc *mailService, repo *mailMemRepo, orgID uuid.UUID) {
	t.Helper()
	key, err := mail.KeyFromSecret(mailTestCfg().TokenKey)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := mail.Encrypt(key, []byte("refresh-token"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	agent, _ := svc.EnsureMailAgent(context.Background(), orgID)
	c := &model.MailConnection{
		OrgID: orgID, GoogleEmail: "org@example.com", RefreshTokenEnc: enc,
		Status: model.MailConnConnected, ConnectedAt: &now, NextSyncAt: &now,
		Scopes: mail.RequestedScopes(),
	}
	if agent != nil {
		c.AgentID = &agent.ID
	}
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureMailAgentCreatesOnce(t *testing.T) {
	svc, _, orgID := setupMail(t, &fakeGmail{}, nil, nil)
	a1, err := svc.EnsureMailAgent(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := svc.EnsureMailAgent(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if a1.ID != a2.ID {
		t.Fatal("expected the same seeded agent")
	}
	if !a1.IsBuiltin(model.BuiltinMail) {
		t.Fatal("missing builtin metadata")
	}
}

func TestAvailableFalseUntilConnected(t *testing.T) {
	svc, _, orgID := setupMail(t, &fakeGmail{email: "a@b.c", tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}}, nil, nil)
	if svc.Available(context.Background(), orgID) {
		t.Fatal("should be false before connect")
	}
	connectOrg(t, svc, svc.repo.(*mailMemRepo), orgID)
	if !svc.Available(context.Background(), orgID) {
		t.Fatal("should be true after connect")
	}
}

func TestApproveSendRefusesWhenNotDraft(t *testing.T) {
	gmail := &fakeGmail{email: "org@example.com", tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}}
	svc, repo, orgID := setupMail(t, gmail, nil, nil)
	connectOrg(t, svc, repo, orgID)
	user := uuid.New()
	d := &model.MailDraft{OrgID: orgID, ThreadID: uuid.New(), Status: model.MailDraftRejected, Body: "x", ToEmail: "a@b.c"}
	if err := repo.UpsertDraft(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveSend(context.Background(), orgID, d.ID, user); !errors.Is(err, ErrMailCannotSend) {
		t.Fatalf("want ErrMailCannotSend, got %v", err)
	}
	if gmail.sendCalls != 0 {
		t.Fatalf("send called %d times", gmail.sendCalls)
	}
}

func TestApproveSendSendsOnlyAfterApprove(t *testing.T) {
	gmail := &fakeGmail{email: "org@example.com", tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}}
	svc, repo, orgID := setupMail(t, gmail, nil, nil)
	connectOrg(t, svc, repo, orgID)
	th := &model.MailThread{OrgID: orgID, GmailThreadID: "gt", Status: model.MailThreadDraftReady, FromEmail: "alex@c.com"}
	if err := repo.UpsertThread(context.Background(), th); err != nil {
		t.Fatal(err)
	}
	conn, _ := repo.GetConnectionByOrg(context.Background(), orgID)
	th.ConnectionID = conn.ID
	_ = repo.UpdateThread(context.Background(), th)
	d := &model.MailDraft{OrgID: orgID, ThreadID: th.ID, Status: model.MailDraftDraft, Body: "Hello", Subject: "Re: hi", ToEmail: "alex@c.com"}
	if err := repo.UpsertDraft(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	out, err := svc.ApproveSend(context.Background(), orgID, d.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != model.MailDraftSent {
		t.Errorf("status %q", out.Status)
	}
	if gmail.sendCalls != 1 {
		t.Fatalf("send calls %d", gmail.sendCalls)
	}
	if out.ApprovedBy == nil || *out.ApprovedBy != user {
		t.Error("missing approved_by")
	}
}

func TestRejectDoesNotSend(t *testing.T) {
	gmail := &fakeGmail{email: "org@example.com", tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}}
	svc, repo, orgID := setupMail(t, gmail, nil, nil)
	connectOrg(t, svc, repo, orgID)
	d := &model.MailDraft{OrgID: orgID, ThreadID: uuid.New(), Status: model.MailDraftDraft, Body: "x", ToEmail: "a@b.c"}
	if err := repo.UpsertDraft(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reject(context.Background(), orgID, d.ID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if gmail.sendCalls != 0 {
		t.Fatal("reject must not send")
	}
	got, _ := repo.GetDraft(context.Background(), d.ID)
	if got.Status != model.MailDraftRejected {
		t.Errorf("status %q", got.Status)
	}
}

func TestProcessCommissionsResearchOnlyWhenFlagged(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "question", NeedsResearch: true, SuggestedAction: "reply", Reason: "facts", TriageLabel: "support", Urgency: "normal",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-research", FromEmail: "alex@c.com", Subject: "Latest k8s?",
			Body: "What is the current version?",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 1 {
		t.Fatalf("research calls %d, want 1", rs.calls)
	}
	listed, _ := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if len(listed.Data) != 1 {
		t.Fatalf("threads %d", len(listed.Data))
	}
	if listed.Data[0].Status != model.MailThreadDraftReady {
		t.Errorf("status %q", listed.Data[0].Status)
	}
	if listed.Data[0].ResearchSummary == nil || !strings.Contains(*listed.Data[0].ResearchSummary, "verified fact") {
		t.Errorf("research summary %+v", listed.Data[0].ResearchSummary)
	}
	if len(rs.lastReq.URLs) != 0 {
		t.Errorf("open-web path must not pass URLs, got %v", rs.lastReq.URLs)
	}
}

func TestProcessSkipsResearchWhenNotNeeded(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "request", NeedsResearch: false, SuggestedAction: "reply", Reason: "thanks", TriageLabel: "general", Urgency: "low",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-plain", FromEmail: "alex@c.com", Subject: "Thanks", Body: "Thanks, confirmed.",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 0 {
		t.Fatalf("research must not run, calls=%d", rs.calls)
	}
}

func TestProcessResearchesPinnedURLsEvenWhenNeedsResearchFalse(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "request", NeedsResearch: false, SuggestedAction: "reply", Reason: "thanks", TriageLabel: "general", Urgency: "low",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-pinned", FromEmail: "alex@c.com", Subject: "Team plan price?",
			Body: "How much is the team plan?",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatalf("connection: %v", err)
	}
	c.WatchKnowledgeURLs = []string{"https://example.com/pricing"}
	c.ResearchFocus = "list prices and SLA"
	c.ReplyInstructions = "Be warm and under 80 words."
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 1 {
		t.Fatalf("research calls %d, want 1 even when needs_research=false", rs.calls)
	}
	if len(rs.lastReq.URLs) != 1 || rs.lastReq.URLs[0] != "https://example.com/pricing" {
		t.Fatalf("URLs %+v", rs.lastReq.URLs)
	}
	if rs.lastReq.Topic != "list prices and SLA" {
		t.Errorf("topic %q", rs.lastReq.Topic)
	}
	if !strings.Contains(rs.lastReq.Context, "Pinned knowledge pages only") {
		t.Errorf("context missing pinned notice:\n%s", rs.lastReq.Context)
	}
	if !strings.Contains(rs.lastReq.Context, "Look for: list prices and SLA") {
		t.Errorf("context missing research_focus:\n%s", rs.lastReq.Context)
	}
	if !strings.Contains(rs.lastReq.Context, "How much is the team plan?") {
		t.Errorf("context missing email body:\n%s", rs.lastReq.Context)
	}
}

func TestProcessSkipsOpenWebResearchWhenNotesPresent(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "question", NeedsResearch: true, SuggestedAction: "reply", Reason: "asks price", TriageLabel: "sales", Urgency: "normal",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-notes", FromEmail: "alex@c.com", Subject: "Mac Studio price?",
			Body: "How much is the Mac Studio?",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	c, _ := repo.GetConnectionByOrg(context.Background(), orgID)
	c.KnowledgeNotes = "Mac Studio M5 Max: $2,499. M5 Ultra: $5,499."
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 0 {
		t.Fatalf("notes answer the mail — open-web research must not run, calls=%d", rs.calls)
	}
	listed, _ := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if len(listed.Data) != 1 || listed.Data[0].Status != model.MailThreadDraftReady {
		t.Fatalf("thread %+v", listed.Data)
	}
}

func TestProcessStillResearchesPinnedURLsAlongsideNotes(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "question", NeedsResearch: false, SuggestedAction: "reply", Reason: "asks price", TriageLabel: "sales", Urgency: "normal",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-notes-pinned", FromEmail: "alex@c.com", Subject: "Team plan?",
			Body: "How much is the team plan?",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	c, _ := repo.GetConnectionByOrg(context.Background(), orgID)
	c.KnowledgeNotes = "Solo plan: £10/user."
	c.WatchKnowledgeURLs = []string{"https://example.com/pricing"}
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 1 {
		t.Fatalf("pinned pages should still be researched on top of notes, calls=%d", rs.calls)
	}
}

func TestProcessIgnoreMailDoesNotResearchEvenWithPinnedURLs(t *testing.T) {
	rs := &fakeResearch{}
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "fyi", NeedsResearch: false, SuggestedAction: "ignore", Reason: "newsletter", TriageLabel: "newsletter", Urgency: "low",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-ignore", FromEmail: "news@list.com", Subject: "This week",
			Body: "Unsubscribe if you no longer want this newsletter. View in browser.",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, rs)
	connectOrg(t, svc, repo, orgID)
	c, _ := repo.GetConnectionByOrg(context.Background(), orgID)
	c.WatchKnowledgeURLs = []string{"https://example.com/pricing"}
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	if rs.calls != 0 {
		t.Fatalf("ignore mail must not research, calls=%d", rs.calls)
	}
	listed, _ := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if len(listed.Data) != 1 || listed.Data[0].Status != model.MailThreadIgnored {
		t.Fatalf("thread %+v", listed.Data)
	}
}

func TestProcessDoesNotIgnoreMailMatchingWatchPrefix(t *testing.T) {
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "fyi", NeedsResearch: false, SuggestedAction: "ignore",
		Reason: "no-reply notification", TriageLabel: "notification", Urgency: "low",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-otp",
			FromEmail:     "noreply@diytaxreturn.co.uk",
			FromName:      "Diy Tax Return",
			Subject:       "[INT] Your DIY Tax Return Verification Code",
			Body:          "Your code is 123456.",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, nil)
	connectOrg(t, svc, repo, orgID)
	c, _ := repo.GetConnectionByOrg(context.Background(), orgID)
	c.WatchSenders = []string{"Balinder Walia", "Sukhvir Singh"}
	c.WatchSubjectPrefixes = []string{"[INT] Your DIY Tax Return Verification Code"}
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	listed, err := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 {
		t.Fatalf("threads %d", len(listed.Data))
	}
	if listed.Data[0].Status == model.MailThreadIgnored {
		t.Fatal("operator-watched subject prefix must not be ignored")
	}
	if listed.Data[0].Status != model.MailThreadDraftReady {
		t.Fatalf("status %q", listed.Data[0].Status)
	}
}

func TestProcessReopensIgnoredThreadWhenWatchPrefixMatches(t *testing.T) {
	class := scriptClass{result: mail.ClassifyResult{
		Intent: "fyi", SuggestedAction: "ignore", Reason: "notification", TriageLabel: "notification",
	}}
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-otp-reopen",
			FromEmail:     "noreply@diytaxreturn.co.uk",
			FromName:      "Diy Tax Return",
			Subject:       "[INT] Your DIY Tax Return Verification Code",
			Body:          "Your code is 123456.",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, class, nil)
	connectOrg(t, svc, repo, orgID)
	c, _ := repo.GetConnectionByOrg(context.Background(), orgID)
	if err := repo.UpsertThread(context.Background(), &model.MailThread{
		OrgID: c.OrgID, ConnectionID: c.ID, Status: model.MailThreadIgnored,
		GmailThreadID: "th-otp-reopen", FromEmail: "noreply@diytaxreturn.co.uk",
		Subject: "[INT] Your DIY Tax Return Verification Code", BodyText: "Your code is 123456.",
	}); err != nil {
		t.Fatal(err)
	}
	c.WatchSubjectPrefixes = []string{"[INT] Your DIY Tax Return Verification Code"}
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncNow(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	listed, err := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].Status == model.MailThreadIgnored {
		t.Fatalf("ignored+watched thread must be reopened, got %+v", listed.Data)
	}
}

func TestUpdateConnectionRoundTripsKnowledgePlaybook(t *testing.T) {
	svc, repo, orgID := setupMail(t, &fakeGmail{}, nil, nil)
	urls := []string{"https://example.com/pricing", "http://docs.example.com/sla"}
	focus := "prices, SLA, refund window"
	style := "Be warm. Under 120 words. Never mention competitors."
	st, err := svc.UpdateConnection(context.Background(), orgID, model.UpdateMailConnectionRequest{
		KnowledgeURLs:     &urls,
		ResearchFocus:     &focus,
		ReplyInstructions: &style,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.KnowledgeURLs) != 2 || st.KnowledgeURLs[0] != "https://example.com/pricing" {
		t.Fatalf("status urls %+v", st.KnowledgeURLs)
	}
	if st.ResearchFocus != focus {
		t.Errorf("focus %q", st.ResearchFocus)
	}
	if st.ReplyInstructions != style {
		t.Errorf("reply %q", st.ReplyInstructions)
	}
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatalf("row: %v", err)
	}
	if len(c.WatchKnowledgeURLs) != 2 {
		t.Fatalf("stored urls %+v", c.WatchKnowledgeURLs)
	}
	if c.ResearchFocus != focus || c.ReplyInstructions != style {
		t.Errorf("stored focus/reply %q / %q", c.ResearchFocus, c.ReplyInstructions)
	}

	st2, err := svc.ConnectionStatus(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if st2.ResearchFocus != focus || st2.ReplyInstructions != style {
		t.Errorf("reload %+v", st2)
	}
	if len(st2.KnowledgeURLs) != 2 {
		t.Fatalf("reload urls %+v", st2.KnowledgeURLs)
	}
}

func TestUpdateConnectionRoundTripsKnowledgeNotes(t *testing.T) {
	svc, repo, orgID := setupMail(t, &fakeGmail{}, nil, nil)
	notes := "  Mac Studio M5 Max: $2,499\nRefunds within 30 days.  "
	st, err := svc.UpdateConnection(context.Background(), orgID, model.UpdateMailConnectionRequest{
		KnowledgeNotes: &notes,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Mac Studio M5 Max: $2,499\nRefunds within 30 days."
	if st.KnowledgeNotes != want {
		t.Errorf("status notes %q", st.KnowledgeNotes)
	}
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatalf("row: %v", err)
	}
	if c.KnowledgeNotes != want {
		t.Errorf("stored notes %q", c.KnowledgeNotes)
	}
}

func TestUpdateConnectionRejectsOversizedKnowledgeNotes(t *testing.T) {
	svc, _, orgID := setupMail(t, &fakeGmail{}, nil, nil)
	big := strings.Repeat("x", mail.MaxKnowledgeNotesLen+1)
	_, err := svc.UpdateConnection(context.Background(), orgID, model.UpdateMailConnectionRequest{
		KnowledgeNotes: &big,
	})
	if !errors.Is(err, mail.ErrKnowledgeNotesTooLong) {
		t.Fatalf("want ErrKnowledgeNotesTooLong, got %v", err)
	}
}

func TestUpdateConnectionRejectsJavascriptKnowledgeURL(t *testing.T) {
	svc, _, orgID := setupMail(t, &fakeGmail{}, nil, nil)
	bad := []string{"javascript:alert(1)"}
	_, err := svc.UpdateConnection(context.Background(), orgID, model.UpdateMailConnectionRequest{
		KnowledgeURLs: &bad,
	})
	if !errors.Is(err, mail.ErrInvalidKnowledgeURL) {
		t.Fatalf("want ErrInvalidKnowledgeURL, got %v", err)
	}
}

func TestOAuthStartURLContainsNoSecrets(t *testing.T) {
	gmail := &fakeGmail{tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r"}}
	svc, _, orgID := setupMail(t, gmail, nil, nil)
	out, err := svc.StartOAuth(context.Background(), orgID, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.AuthorizationURL, "csec") || strings.Contains(out.AuthorizationURL, "client_secret") {
		t.Errorf("auth URL leaked secret: %s", out.AuthorizationURL)
	}
	if !strings.Contains(out.AuthorizationURL, "access_type=offline") {
		t.Error("missing offline access")
	}
}

func TestUpdateConnectionSavesRulesWhileDisconnected(t *testing.T) {
	svc, repo, orgID := setupMail(t, &fakeGmail{}, nil, nil)
	st, err := svc.UpdateConnection(context.Background(), orgID, model.UpdateMailConnectionRequest{
		Rules: &model.MailWatchRules{
			Senders:         []string{"ops@example.com"},
			SubjectPrefixes: []string{"[support]"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Connected {
		t.Fatal("saving rules must not mark the mailbox connected")
	}
	if len(st.Rules.Senders) != 1 || st.Rules.Senders[0] != "ops@example.com" {
		t.Fatalf("senders %+v", st.Rules.Senders)
	}
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatalf("expected a disconnected connection row: %v", err)
	}
	if c.Status != model.MailConnDisconnected {
		t.Errorf("status %q", c.Status)
	}
	if c.Scopes == nil {
		t.Fatal("scopes must be an empty slice, not nil (postgres text[] NOT NULL)")
	}
}

func TestAvailableAndConnectionStatusWhenStatusError(t *testing.T) {
	svc, repo, orgID := setupMail(t, &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
	}, nil, nil)
	connectOrg(t, svc, repo, orgID)
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatalf("connection: %v", err)
	}
	c.Status = model.MailConnError
	msg := "mail: gmail api: decode (json: cannot unmarshal string)"
	c.StatusError = &msg
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !svc.Available(context.Background(), orgID) {
		t.Fatal("error status with a refresh token must still be available")
	}
	st, err := svc.ConnectionStatus(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Connected {
		t.Fatal("ConnectionStatus.Connected must stay true so the UI does not flip to Connect Gmail")
	}
	if st.Status != model.MailConnError {
		t.Errorf("status %q", st.Status)
	}
	if st.StatusError != msg {
		t.Errorf("status_error %q", st.StatusError)
	}
	if st.Email != "org@example.com" {
		t.Errorf("email %q", st.Email)
	}
}

func TestClaimDueConnectionsClaimsErrorRows(t *testing.T) {
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-error-claim", FromEmail: "alex@c.com", Subject: "Hello", Body: "Hi",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, nil, nil)
	connectOrg(t, svc, repo, orgID)
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatalf("connection: %v", err)
	}
	c.Status = model.MailConnError
	now := time.Now().Add(-time.Second)
	c.NextSyncAt = &now
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessDueSyncs(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	if gmail.listCalls < 1 {
		t.Fatal("ClaimDueConnections must pick up status=error rows")
	}
	listed, err := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 {
		t.Fatalf("threads %d, want 1 after claiming an error mailbox", len(listed.Data))
	}
}

func TestEnqueueSyncClearsLease(t *testing.T) {
	svc, repo, orgID := setupMail(t, &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
	}, nil, nil)
	connectOrg(t, svc, repo, orgID)
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatalf("connection: %v", err)
	}
	until := time.Now().Add(2 * time.Minute)
	c.SyncLeaseUntil = &until
	c.Status = model.MailConnError
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnqueueSync(context.Background(), orgID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || got == nil {
		t.Fatalf("reload: %v", err)
	}
	if got.SyncLeaseUntil != nil {
		t.Fatal("EnqueueSync must persist sync_lease_until = NULL")
	}
}

func TestProcessDueSyncsClearsLeaseOnFailure(t *testing.T) {
	gmail := &fakeGmail{
		email:   "org@example.com",
		tokens:  mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		listErr: errors.New("mail: gmail api: decode (json: cannot unmarshal string)"),
	}
	svc, repo, orgID := setupMail(t, gmail, nil, nil)
	connectOrg(t, svc, repo, orgID)
	if err := svc.ProcessDueSyncs(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || got == nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status != model.MailConnError {
		t.Errorf("status %q", got.Status)
	}
	if got.SyncLeaseUntil != nil {
		t.Fatal("failed sync must persist sync_lease_until = NULL")
	}
}

func TestSyncInboxPullsWithoutDrafting(t *testing.T) {
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
		messages: []mail.InboxMessage{{
			GmailThreadID: "th-sync-inbox", FromEmail: "alex@c.com", Subject: "Hello", Body: "Hi",
		}},
	}
	svc, repo, orgID := setupMail(t, gmail, nil, nil)
	connectOrg(t, svc, repo, orgID)
	out, err := svc.SyncInbox(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Listed != 1 || out.Ingested != 1 {
		t.Fatalf("listed=%d ingested=%d", out.Listed, out.Ingested)
	}
	if !strings.Contains(out.Query, "in:inbox") {
		t.Fatalf("query %q", out.Query)
	}
	listed, err := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].Status != model.MailThreadNew {
		t.Fatalf("want one new thread, got %+v", listed.Data)
	}
	if drafts, _ := repo.ListDraftsByStatus(context.Background(), orgID, model.MailDraftDraft, model.PaginationParams{}); drafts != nil && drafts.Total != 0 {
		t.Fatal("SyncInbox must not draft; reconciler does that")
	}
}

func TestSyncInboxQuotesDisplayNameSenders(t *testing.T) {
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
	}
	svc, repo, orgID := setupMail(t, gmail, nil, nil)
	connectOrg(t, svc, repo, orgID)
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatal(err)
	}
	c.WatchSenders = []string{"Balinder Walia"}
	if err := repo.UpsertConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	out, err := svc.SyncInbox(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if gmail.lastQuery != `in:inbox newer_than:7d (from:"Balinder Walia")` {
		t.Fatalf("query %q", gmail.lastQuery)
	}
	if out.Listed != 0 {
		t.Fatalf("listed %d", out.Listed)
	}
}

func TestProcessDueSyncsProcessesQueuedNewThreads(t *testing.T) {
	gmail := &fakeGmail{
		email:  "org@example.com",
		tokens: mail.TokenSet{AccessToken: "a", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)},
	}
	svc, repo, orgID := setupMail(t, gmail, nil, nil)
	connectOrg(t, svc, repo, orgID)
	c, err := repo.GetConnectionByOrg(context.Background(), orgID)
	if err != nil || c == nil {
		t.Fatal(err)
	}
	if err := repo.UpsertThread(context.Background(), &model.MailThread{
		OrgID: c.OrgID, ConnectionID: c.ID, Status: model.MailThreadNew,
		GmailThreadID: "queued-th", FromEmail: "alex@c.com", Subject: "Queued", BodyText: "Hi",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessDueSyncs(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	listed, err := repo.ListThreads(context.Background(), orgID, model.PaginationParams{})
	if err != nil || len(listed.Data) != 1 {
		t.Fatalf("threads: %v %+v", err, listed)
	}
	if listed.Data[0].Status != model.MailThreadDraftReady && listed.Data[0].Status != model.MailThreadIgnored {
		t.Fatalf("queued thread status %q", listed.Data[0].Status)
	}
}
