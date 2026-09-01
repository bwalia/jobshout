package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
)

var (
	ErrMailNotConfigured       = errors.New("mail: Gmail OAuth is not configured on this server")
	ErrMailNotConnected        = errors.New("mail: Gmail is not connected")
	ErrMailNotFound            = errors.New("mail: not found")
	ErrMailCannotSend          = errors.New("mail: send requires an approved draft")
	ErrMailDraftNotEditable    = errors.New("mail: draft cannot be edited in its current state")
	ErrMailInvalidKnowledgeURL = mail.ErrInvalidKnowledgeURL
)

// MailService is the org mailbox specialist: connect, sync, draft, approve-to-send.
type MailService interface {
	EnsureMailAgent(ctx context.Context, orgID uuid.UUID) (*model.Agent, error)
	Configured() bool
	Available(ctx context.Context, orgID uuid.UUID) bool

	ConnectionStatus(ctx context.Context, orgID uuid.UUID) (*model.MailConnectionStatus, error)
	StartOAuth(ctx context.Context, orgID, userID uuid.UUID) (*model.MailOAuthStartResponse, error)
	CompleteOAuth(ctx context.Context, state, code string) error
	Disconnect(ctx context.Context, orgID uuid.UUID) error
	UpdateConnection(ctx context.Context, orgID uuid.UUID, req model.UpdateMailConnectionRequest) (*model.MailConnectionStatus, error)

	SyncNow(ctx context.Context, orgID uuid.UUID) error
	EnqueueSync(ctx context.Context, orgID uuid.UUID) error
	ProcessDueSyncs(ctx context.Context, limit int) error
	// BindTasks lets a launched sync update its Task Manager card. Optional.
	BindTasks(tasks TaskService)
	// BindLaunchTask records which board card this org's next sync should update.
	// Periodic reconciler syncs with no binding leave the board alone.
	BindLaunchTask(orgID, taskID uuid.UUID)

	ListThreads(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailThread], error)
	GetThread(ctx context.Context, orgID, threadID uuid.UUID) (*model.MailThreadDetail, error)
	ListPendingDrafts(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailDraft], error)

	UpdateDraft(ctx context.Context, orgID, draftID uuid.UUID, req model.UpdateMailDraftRequest) (*model.MailDraft, error)
	ApproveSend(ctx context.Context, orgID, draftID, userID uuid.UUID) (*model.MailDraft, error)
	Reject(ctx context.Context, orgID, draftID, userID uuid.UUID) (*model.MailDraft, error)

	// Local MAIL_SIMULATE only. Production Gmail is never used when these run.
	SimulateEnabled() bool
	ConnectSimulated(ctx context.Context, orgID uuid.UUID) (*model.MailConnectionStatus, error)
	PushSimulatedInbox(msgs []mail.InboxMessage) error
}

type mailService struct {
	repo       repository.MailRepository
	agentRepo  repository.AgentRepository
	gmail      mail.GmailAPI
	classifier mail.Classifier
	drafter    mail.Drafter
	research   ResearchService
	tasks      TaskService
	launchMu   sync.Mutex
	launchTask map[uuid.UUID]uuid.UUID // orgID → board task for the next sync
	cfg        mail.Config
	key        []byte
	logger     *zap.Logger
}

func (s *mailService) BindTasks(tasks TaskService) {
	if s != nil {
		s.tasks = tasks
	}
}

func (s *mailService) BindLaunchTask(orgID, taskID uuid.UUID) {
	if s == nil {
		return
	}
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	if s.launchTask == nil {
		s.launchTask = map[uuid.UUID]uuid.UUID{}
	}
	if taskID == uuid.Nil {
		delete(s.launchTask, orgID)
		return
	}
	s.launchTask[orgID] = taskID
}

func (s *mailService) peekLaunchTask(orgID uuid.UUID) uuid.UUID {
	if s == nil {
		return uuid.Nil
	}
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	return s.launchTask[orgID]
}

func (s *mailService) clearLaunchTask(orgID uuid.UUID) {
	s.BindLaunchTask(orgID, uuid.Nil)
}

// NewMailService wires the Mail Agent. gmail/classifier/drafter may be fakes in tests.
func NewMailService(
	repo repository.MailRepository,
	agentRepo repository.AgentRepository,
	gmail mail.GmailAPI,
	classifier mail.Classifier,
	drafter mail.Drafter,
	researchSvc ResearchService,
	cfg mail.Config,
	logger *zap.Logger,
) MailService {
	if logger == nil {
		logger = zap.NewNop()
	}
	var key []byte
	if cfg.TokenKey != "" {
		k, err := mail.KeyFromSecret(cfg.TokenKey)
		if err != nil {
			logger.Warn("mail: token key unusable", zap.Error(err))
		} else {
			key = k
		}
	}
	return &mailService{
		repo: repo, agentRepo: agentRepo, gmail: gmail,
		classifier: classifier, drafter: drafter, research: researchSvc,
		cfg: cfg, key: key, logger: logger,
	}
}

func mailAgentSeed(orgID uuid.UUID) *model.Agent {
	desc := "Watches the organisation Gmail inbox, drafts replies, and hands research to the Research Agent. Nothing is sent until a human approves."
	prompt := "You are the Mail Agent. You triage the organisation inbox, draft replies, and never send until a human approves. You never claim a message was sent unless the send API succeeded after approval. Work that needs facts is handed to the Research Agent — you do not invent citations."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameMail,
		Role:         "Mail",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinMail},
	}
}

func (s *mailService) EnsureMailAgent(ctx context.Context, orgID uuid.UUID) (*model.Agent, error) {
	existing, err := s.agentRepo.FindBuiltin(ctx, orgID, model.BuiltinMail)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	agent := mailAgentSeed(orgID)
	if err := s.agentRepo.Create(ctx, agent); err != nil {
		return nil, fmt.Errorf("mail_svc: seed mail agent: %w", err)
	}
	s.logger.Info("mail: seeded built-in Mail Agent",
		zap.String("org_id", orgID.String()), zap.String("agent_id", agent.ID.String()))
	return agent, nil
}

func (s *mailService) Configured() bool {
	return s != nil && s.cfg.Configured() && len(s.key) == 32 && s.gmail != nil
}

func (s *mailService) Available(ctx context.Context, orgID uuid.UUID) bool {
	if !s.Configured() {
		return false
	}
	c, err := s.repo.GetConnectionByOrg(ctx, orgID)
	return err == nil && mailboxLinked(c)
}

func mailboxLinked(c *model.MailConnection) bool {
	return c != nil && len(c.RefreshTokenEnc) > 0 && c.Status != model.MailConnDisconnected
}

func (s *mailService) ConnectionStatus(ctx context.Context, orgID uuid.UUID) (*model.MailConnectionStatus, error) {
	st := &model.MailConnectionStatus{
		Configured:        s.Configured(),
		Status:            model.MailConnDisconnected,
		ScopesDocumented:  mail.ScopeDocs(),
		Rules:             model.MailWatchRules{Labels: []string{}, Senders: []string{}, SubjectPrefixes: []string{}},
		KnowledgeURLs:     []string{},
		KnowledgeNotes:    "",
		ResearchFocus:     "",
		ReplyInstructions: "",
	}
	if agent, err := s.EnsureMailAgent(ctx, orgID); err == nil && agent != nil {
		st.AgentID = &agent.ID
	}
	c, err := s.repo.GetConnectionByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return st, nil
	}
	st.Connected = mailboxLinked(c)
	st.Email = c.GoogleEmail
	st.Status = c.Status
	if c.StatusError != nil {
		st.StatusError = mail.Redact(*c.StatusError)
	}
	st.AllowMailboxMutations = c.AllowMailboxMutations
	st.Rules = model.MailWatchRules{
		Labels: c.WatchLabels, Senders: c.WatchSenders, SubjectPrefixes: c.WatchSubjectPrefixes,
	}
	st.KnowledgeURLs = c.WatchKnowledgeURLs
	if st.KnowledgeURLs == nil {
		st.KnowledgeURLs = []string{}
	}
	st.KnowledgeNotes = c.KnowledgeNotes
	st.ResearchFocus = c.ResearchFocus
	st.ReplyInstructions = c.ReplyInstructions
	st.Scopes = c.Scopes
	st.LastSyncAt = c.LastSyncAt
	st.ConnectedAt = c.ConnectedAt
	st.AgentID = c.AgentID
	if st.AgentID == nil {
		if agent, err := s.EnsureMailAgent(ctx, orgID); err == nil && agent != nil {
			st.AgentID = &agent.ID
		}
	}
	return st, nil
}

func (s *mailService) StartOAuth(ctx context.Context, orgID, userID uuid.UUID) (*model.MailOAuthStartResponse, error) {
	if !s.Configured() {
		return nil, ErrMailNotConfigured
	}
	if _, err := s.EnsureMailAgent(ctx, orgID); err != nil {
		s.logger.Warn("mail: could not seed agent before oauth", zap.Error(err))
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("mail_svc: state: %w", err)
	}
	state := hex.EncodeToString(buf)
	if err := s.repo.PutOAuthState(ctx, state, orgID, userID, time.Now().Add(10*time.Minute)); err != nil {
		return nil, err
	}
	return &model.MailOAuthStartResponse{
		AuthorizationURL: mail.AuthURL(s.cfg.ClientID, s.cfg.RedirectURL, state),
	}, nil
}

func (s *mailService) CompleteOAuth(ctx context.Context, state, code string) error {
	if !s.Configured() {
		return ErrMailNotConfigured
	}
	orgID, _, err := s.repo.ConsumeOAuthState(ctx, state)
	if err != nil {
		return err
	}
	tokens, err := s.gmail.ExchangeCode(ctx, code, s.cfg.RedirectURL, s.cfg.ClientID, s.cfg.ClientSecret)
	if err != nil {
		return mail.RedactErr(err)
	}
	if tokens.RefreshToken == "" {
		return fmt.Errorf("mail: Google did not return a refresh token; reconnect and grant consent")
	}
	enc, err := mail.Encrypt(s.key, []byte(tokens.RefreshToken))
	if err != nil {
		return err
	}
	email, err := s.gmail.Profile(ctx, tokens.AccessToken)
	if err != nil {
		return mail.RedactErr(err)
	}
	agent, _ := s.EnsureMailAgent(ctx, orgID)
	now := time.Now()
	conn := &model.MailConnection{
		OrgID:                orgID,
		GoogleEmail:          email,
		RefreshTokenEnc:      enc,
		TokenExpiry:          &tokens.Expiry,
		Scopes:               mail.RequestedScopes(),
		Status:               model.MailConnConnected,
		ConnectedAt:          &now,
		NextSyncAt:           &now,
		WatchLabels:          []string{},
		WatchSenders:         []string{},
		WatchSubjectPrefixes: []string{},
		WatchKnowledgeURLs:   []string{},
	}
	if agent != nil {
		conn.AgentID = &agent.ID
	}
	if existing, _ := s.repo.GetConnectionByOrg(ctx, orgID); existing != nil {
		conn.AllowMailboxMutations = existing.AllowMailboxMutations
		conn.WatchLabels = existing.WatchLabels
		conn.WatchSenders = existing.WatchSenders
		conn.WatchSubjectPrefixes = existing.WatchSubjectPrefixes
		conn.WatchKnowledgeURLs = existing.WatchKnowledgeURLs
		conn.ResearchFocus = existing.ResearchFocus
		conn.ReplyInstructions = existing.ReplyInstructions
	}
	if err := s.repo.UpsertConnection(ctx, conn); err != nil {
		return err
	}
	s.logger.Info("mail: gmail connected",
		zap.String("org_id", orgID.String()),
		zap.String("email", email))
	return nil
}

func (s *mailService) Disconnect(ctx context.Context, orgID uuid.UUID) error {
	if err := s.repo.Disconnect(ctx, orgID); err != nil {
		return err
	}
	s.logger.Info("mail: gmail disconnected", zap.String("org_id", orgID.String()))
	return nil
}

func (s *mailService) UpdateConnection(ctx context.Context, orgID uuid.UUID, req model.UpdateMailConnectionRequest) (*model.MailConnectionStatus, error) {
	c, err := s.repo.GetConnectionByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		agent, aerr := s.EnsureMailAgent(ctx, orgID)
		if aerr != nil {
			return nil, aerr
		}
		c = &model.MailConnection{
			OrgID: orgID, Status: model.MailConnDisconnected,
			Scopes:               []string{},
			WatchLabels:          []string{},
			WatchSenders:         []string{},
			WatchSubjectPrefixes: []string{},
			WatchKnowledgeURLs:   []string{},
		}
		if agent != nil {
			c.AgentID = &agent.ID
		}
		if err := s.repo.UpsertConnection(ctx, c); err != nil {
			return nil, err
		}
	}
	if req.AllowMailboxMutations != nil {
		c.AllowMailboxMutations = *req.AllowMailboxMutations
	}
	if req.Rules != nil {
		c.WatchLabels = req.Rules.Labels
		c.WatchSenders = req.Rules.Senders
		c.WatchSubjectPrefixes = req.Rules.SubjectPrefixes
	}
	if req.KnowledgeURLs != nil {
		urls, err := mail.SanitizeKnowledgeURLs(*req.KnowledgeURLs)
		if err != nil {
			return nil, err
		}
		c.WatchKnowledgeURLs = urls
	}
	if req.KnowledgeNotes != nil {
		notes, err := mail.SanitizeKnowledgeNotes(*req.KnowledgeNotes)
		if err != nil {
			return nil, err
		}
		c.KnowledgeNotes = notes
	}
	if req.ResearchFocus != nil {
		c.ResearchFocus = strings.TrimSpace(*req.ResearchFocus)
	}
	if req.ReplyInstructions != nil {
		c.ReplyInstructions = strings.TrimSpace(*req.ReplyInstructions)
	}
	if err := s.repo.UpdateConnectionMeta(ctx, c); err != nil {
		return nil, err
	}
	return s.ConnectionStatus(ctx, orgID)
}

func (s *mailService) EnqueueSync(ctx context.Context, orgID uuid.UUID) error {
	if !s.Available(ctx, orgID) {
		if !s.Configured() {
			return ErrMailNotConfigured
		}
		return ErrMailNotConnected
	}
	c, err := s.repo.GetConnectionByOrg(ctx, orgID)
	if err != nil {
		return err
	}
	now := time.Now()
	c.NextSyncAt = &now
	c.SyncLeaseUntil = nil
	return s.repo.UpsertConnection(ctx, c)
}

func (s *mailService) SyncNow(ctx context.Context, orgID uuid.UUID) error {
	if err := s.EnqueueSync(ctx, orgID); err != nil {
		return err
	}
	c, err := s.repo.GetConnectionByOrg(ctx, orgID)
	if err != nil {
		return err
	}
	syncErr := s.syncConnection(ctx, c)
	next := time.Now().Add(s.pollInterval())
	_ = s.repo.MarkSynced(ctx, c.ID, time.Now(), next)
	return syncErr
}

func (s *mailService) ProcessDueSyncs(ctx context.Context, limit int) error {
	if s.gmail == nil || len(s.key) != 32 {
		return nil
	}
	conns, err := s.repo.ClaimDueConnections(ctx, limit, 2*time.Minute)
	if err != nil {
		return err
	}
	for i := range conns {
		c := &conns[i]
		next := time.Now().Add(s.pollInterval())
		if err := s.syncConnection(ctx, c); err != nil {
			s.logger.Warn("mail: sync failed",
				zap.String("org_id", c.OrgID.String()),
				zap.Error(mail.RedactErr(err)))
			msg := mail.Redact(err.Error())
			c.Status = model.MailConnError
			c.StatusError = &msg
			c.NextSyncAt = &next
			c.SyncLeaseUntil = nil
			_ = s.repo.UpsertConnection(ctx, c)
			continue
		}
		_ = s.repo.MarkSynced(ctx, c.ID, time.Now(), next)
	}
	return nil
}

func (s *mailService) pollInterval() time.Duration {
	if s.cfg.PollInterval > 0 {
		return s.cfg.PollInterval
	}
	return 5 * time.Minute
}

func (s *mailService) syncConnection(ctx context.Context, c *model.MailConnection) error {
	access, err := s.accessToken(ctx, c)
	if err != nil {
		return err
	}
	agent, _ := s.EnsureMailAgent(ctx, c.OrgID)
	query := mail.RulesQuery(c.WatchLabels, c.WatchSenders, c.WatchSubjectPrefixes)
	msgs, err := s.gmail.ListMessages(ctx, access, query, 25)
	if err != nil {
		return mail.RedactErr(err)
	}
	launchTask := s.peekLaunchTask(c.OrgID)
	drafted := 0
	for _, msg := range msgs {
		ok, err := s.ingestAndProcess(ctx, c, agent, msg, launchTask)
		if err != nil {
			s.logger.Warn("mail: thread processing failed",
				zap.String("gmail_thread_id", msg.GmailThreadID),
				zap.Error(mail.RedactErr(err)))
			continue
		}
		if ok {
			drafted++
		}
	}
	if launchTask != uuid.Nil && drafted == 0 {
		s.notifyMailTask(ctx, launchTask,
			"Mailbox sync finished. No new drafts. Nothing is sent until you Approve.",
			"done")
	}
	if launchTask != uuid.Nil {
		s.clearLaunchTask(c.OrgID)
	}
	return nil
}

func (s *mailService) ingestAndProcess(ctx context.Context, c *model.MailConnection, agent *model.Agent, msg mail.InboxMessage, launchTask uuid.UUID) (bool, error) {
	existing, err := s.repo.GetThreadByGmailID(ctx, c.OrgID, msg.GmailThreadID)
	if err != nil {
		return false, err
	}
	if existing != nil && existing.Status != model.MailThreadNew && existing.Status != model.MailThreadFailed {
		return false, nil
	}
	th := existing
	if th == nil {
		th = &model.MailThread{
			OrgID: c.OrgID, ConnectionID: c.ID, Status: model.MailThreadNew,
		}
		if agent != nil {
			th.AgentID = &agent.ID
		}
	}
	th.GmailThreadID = msg.GmailThreadID
	th.GmailMessageID = msg.GmailMessageID
	th.FromEmail = msg.FromEmail
	th.FromName = msg.FromName
	th.ToEmail = msg.ToEmail
	th.Subject = msg.Subject
	th.Snippet = msg.Snippet
	th.BodyText = msg.Body
	th.MessageIDHeader = msg.MessageIDHeader
	th.ReferencesHeader = msg.ReferencesHeader
	if !msg.ReceivedAt.IsZero() {
		t := msg.ReceivedAt
		th.ReceivedAt = &t
	}
	if th.Status == "" {
		th.Status = model.MailThreadNew
	}
	if err := s.repo.UpsertThread(ctx, th); err != nil {
		return false, err
	}
	if err := s.processThread(ctx, th, msg, c, launchTask); err != nil {
		return false, err
	}
	return th.Status == model.MailThreadDraftReady, nil
}

func (s *mailService) processThread(ctx context.Context, th *model.MailThread, msg mail.InboxMessage, c *model.MailConnection, launchTask uuid.UUID) error {
	th.Status = model.MailThreadClassifying
	_ = s.repo.UpdateThread(ctx, th)

	class, err := s.classifier.Classify(ctx, msg)
	if err != nil {
		return s.failThread(ctx, th, err)
	}
	th.Classification = &class
	th.NeedsResearch = class.NeedsResearch
	if class.SuggestedAction == "ignore" {
		th.Status = model.MailThreadIgnored
		return s.repo.UpdateThread(ctx, th)
	}

	var notes string
	if c != nil {
		notes = strings.TrimSpace(c.KnowledgeNotes)
	}
	pinnedReq, pinned := mailKnowledgeRequest(c, msg)
	// Operator-written knowledge notes are the primary source when present:
	// they already answer the mail, so the open-web fallback is skipped.
	// Pinned pages and inbound links still add findings on top of the notes.
	wantResearch := pinned || (notes == "" && class.NeedsResearch)

	var brief *research.Brief
	if wantResearch && s.research != nil && s.research.Available() {
		th.Status = model.MailThreadResearching
		_ = s.repo.UpdateThread(ctx, th)
		req := pinnedReq
		if !pinned {
			topic := strings.TrimSpace(msg.Subject)
			if topic == "" {
				topic = "inbound email"
			}
			req = research.Request{
				Topic:   topic,
				Context: "Write a factual briefing that can be used in a short professional email reply.\n\n" + strings.TrimSpace(msg.Body),
			}
		}
		b, rerr := s.research.Research(ctx, th.OrgID, req, nil)
		if rerr != nil {
			s.logger.Warn("mail: research handoff failed, drafting without it", zap.Error(rerr))
		} else {
			brief = b
			sum := b.Summary
			if sum == "" && len(b.Warnings) > 0 {
				sum = strings.Join(b.Warnings, " ")
			}
			th.ResearchSummary = &sum
			id := uuid.New()
			th.ResearchBriefID = &id
			if raw, merr := json.Marshal(b); merr == nil {
				th.ResearchFindings = raw
			}
		}
	} else if wantResearch {
		s.logger.Info("mail: research requested but Research Agent is unavailable; drafting without it")
	}

	opts := mail.DraftOptions{KnowledgeNotes: notes}
	if c != nil {
		opts.ReplyInstructions = c.ReplyInstructions
	}
	if pinned {
		opts.PinnedKnowledge = true
	}
	draft, err := s.drafter.Draft(ctx, msg, class, brief, opts)
	if err != nil {
		return s.failThread(ctx, th, err)
	}
	row := &model.MailDraft{
		OrgID:    th.OrgID,
		ThreadID: th.ID,
		AgentID:  th.AgentID,
		Status:   model.MailDraftDraft,
		Subject:  draft.Subject,
		Body:     draft.Body,
		ToEmail:  draft.To,
		CCEmail:  draft.CC,
	}
	if th.ResearchBriefID != nil {
		row.ResearchBriefID = th.ResearchBriefID
	}
	if err := s.repo.UpsertDraft(ctx, row); err != nil {
		return s.failThread(ctx, th, err)
	}
	th.Status = model.MailThreadDraftReady
	th.ErrorMessage = nil
	if err := s.repo.UpdateThread(ctx, th); err != nil {
		return err
	}
	s.notifyMailTask(ctx, launchTask, fmt.Sprintf(
		"Draft ready: %s\n\nOpen /panel/task-manager?agent=mail&thread=%s\nNothing is sent until you Approve.",
		draft.Subject, th.ID), "review")
	return nil
}

func (s *mailService) notifyMailTask(ctx context.Context, taskID uuid.UUID, note, status string) {
	if s == nil || s.tasks == nil || taskID == uuid.Nil {
		return
	}
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	n := note
	if task.Description != nil && strings.TrimSpace(*task.Description) != "" {
		n = strings.TrimSpace(*task.Description) + "\n\n" + note
	}
	_, _ = s.tasks.Update(ctx, taskID, model.UpdateTaskRequest{Description: &n})
	if status != "" {
		_ = s.tasks.Transition(ctx, taskID, status, nil)
	}
}

func (s *mailService) failThread(ctx context.Context, th *model.MailThread, err error) error {
	msg := mail.Redact(err.Error())
	th.Status = model.MailThreadFailed
	th.ErrorMessage = &msg
	_ = s.repo.UpdateThread(ctx, th)
	return err
}

func (s *mailService) ListThreads(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailThread], error) {
	return s.repo.ListThreads(ctx, orgID, pagination)
}

func (s *mailService) GetThread(ctx context.Context, orgID, threadID uuid.UUID) (*model.MailThreadDetail, error) {
	th, err := s.repo.GetThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if th == nil || th.OrgID != orgID {
		return nil, ErrMailNotFound
	}
	d, err := s.repo.GetDraftByThread(ctx, th.ID)
	if err != nil {
		return nil, err
	}
	return &model.MailThreadDetail{Thread: *th, Draft: d}, nil
}

func (s *mailService) ListPendingDrafts(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailDraft], error) {
	return s.repo.ListDraftsByStatus(ctx, orgID, model.MailDraftDraft, pagination)
}

func (s *mailService) UpdateDraft(ctx context.Context, orgID, draftID uuid.UUID, req model.UpdateMailDraftRequest) (*model.MailDraft, error) {
	d, err := s.ownedDraft(ctx, orgID, draftID)
	if err != nil {
		return nil, err
	}
	if d.Status != model.MailDraftDraft {
		return nil, ErrMailDraftNotEditable
	}
	if req.Body != nil {
		d.Body = *req.Body
	}
	if req.Subject != nil {
		d.Subject = *req.Subject
	}
	if req.CCEmail != nil {
		d.CCEmail = *req.CCEmail
	}
	if err := s.repo.UpdateDraft(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *mailService) ApproveSend(ctx context.Context, orgID, draftID, userID uuid.UUID) (*model.MailDraft, error) {
	d, err := s.ownedDraft(ctx, orgID, draftID)
	if err != nil {
		return nil, err
	}
	if d.Status != model.MailDraftDraft && d.Status != model.MailDraftApproved {
		return nil, ErrMailCannotSend
	}
	th, err := s.repo.GetThread(ctx, d.ThreadID)
	if err != nil {
		return nil, err
	}
	if th == nil || th.OrgID != orgID {
		return nil, ErrMailNotFound
	}
	c, err := s.repo.GetConnectionByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if c == nil || !mailboxLinked(c) {
		return nil, ErrMailNotConnected
	}

	now := time.Now()
	d.Status = model.MailDraftApproved
	d.ApprovedBy = &userID
	d.ApprovedAt = &now
	if err := s.repo.UpdateDraft(ctx, d); err != nil {
		return nil, err
	}

	access, err := s.accessToken(ctx, c)
	if err != nil {
		return nil, mail.RedactErr(err)
	}
	refs := th.ReferencesHeader
	if th.MessageIDHeader != "" {
		if refs == "" {
			refs = th.MessageIDHeader
		} else if !strings.Contains(refs, th.MessageIDHeader) {
			refs = refs + " " + th.MessageIDHeader
		}
	}
	gmailID, err := s.gmail.Send(ctx, access, mail.OutboundMessage{
		From:       c.GoogleEmail,
		To:         d.ToEmail,
		CC:         d.CCEmail,
		Subject:    d.Subject,
		Body:       d.Body,
		InReplyTo:  th.MessageIDHeader,
		References: refs,
		ThreadID:   th.GmailThreadID,
	})
	if err != nil {
		return nil, mail.RedactErr(err)
	}
	sent := time.Now()
	d.Status = model.MailDraftSent
	d.GmailMessageID = &gmailID
	d.SentAt = &sent
	if err := s.repo.UpdateDraft(ctx, d); err != nil {
		return nil, err
	}
	th.Status = model.MailThreadSent
	_ = s.repo.UpdateThread(ctx, th)
	s.logger.Info("mail: sent after approval",
		zap.String("draft_id", d.ID.String()),
		zap.String("approved_by", userID.String()))
	return d, nil
}

func (s *mailService) Reject(ctx context.Context, orgID, draftID, userID uuid.UUID) (*model.MailDraft, error) {
	d, err := s.ownedDraft(ctx, orgID, draftID)
	if err != nil {
		return nil, err
	}
	if d.Status == model.MailDraftSent {
		return nil, ErrMailCannotSend
	}
	now := time.Now()
	d.Status = model.MailDraftRejected
	d.RejectedBy = &userID
	d.RejectedAt = &now
	if err := s.repo.UpdateDraft(ctx, d); err != nil {
		return nil, err
	}
	if th, err := s.repo.GetThread(ctx, d.ThreadID); err == nil && th != nil && th.OrgID == orgID {
		th.Status = model.MailThreadRejected
		_ = s.repo.UpdateThread(ctx, th)
	}
	return d, nil
}

func (s *mailService) ownedDraft(ctx context.Context, orgID, draftID uuid.UUID) (*model.MailDraft, error) {
	d, err := s.repo.GetDraft(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if d == nil || d.OrgID != orgID {
		return nil, ErrMailNotFound
	}
	return d, nil
}

func (s *mailService) accessToken(ctx context.Context, c *model.MailConnection) (string, error) {
	if len(c.RefreshTokenEnc) == 0 {
		return "", ErrMailNotConnected
	}
	plain, err := mail.Decrypt(s.key, c.RefreshTokenEnc)
	if err != nil {
		return "", err
	}
	ts, err := s.gmail.Refresh(ctx, string(plain), s.cfg.ClientID, s.cfg.ClientSecret)
	if err != nil {
		return "", mail.RedactErr(err)
	}
	if ts.RefreshToken != "" && ts.RefreshToken != string(plain) {
		enc, err := mail.Encrypt(s.key, []byte(ts.RefreshToken))
		if err == nil {
			c.RefreshTokenEnc = enc
			_ = s.repo.UpsertConnection(ctx, c)
		}
	}
	return ts.AccessToken, nil
}

// mailKnowledgeRequest builds a pinned-URL research.Request when the mailbox
// has knowledge pages or the inbound mail itself contains http(s) links.
// ok is false when the merged list is empty (open-web path).
func mailKnowledgeRequest(c *model.MailConnection, msg mail.InboxMessage) (research.Request, bool) {
	var playbook []string
	var focus string
	if c != nil {
		playbook = c.WatchKnowledgeURLs
		focus = strings.TrimSpace(c.ResearchFocus)
	}
	inbound := mail.ExtractInboundURLs(msg.Subject, msg.Body)
	urls := mail.MergeKnowledgeURLs(playbook, inbound)
	if len(urls) == 0 {
		return research.Request{}, false
	}
	subject := strings.TrimSpace(msg.Subject)
	body := strings.TrimSpace(msg.Body)
	topic := focus
	if topic == "" {
		topic = subject
	}
	if topic == "" {
		topic = "inbound email"
	}
	var b strings.Builder
	switch {
	case len(playbook) > 0 && len(inbound) == 0:
		b.WriteString("Pinned knowledge pages only. Do not use other sources.")
	case len(playbook) > 0:
		b.WriteString("Read the pinned knowledge pages and the links in this email. Do not use other sources.")
	default:
		b.WriteString("Read the links in this inbound email. Do not use other sources.")
	}
	if focus != "" {
		b.WriteString("\nLook for: ")
		b.WriteString(focus)
	}
	b.WriteString("\nInbound email:\n")
	b.WriteString(subject)
	b.WriteString("\n")
	b.WriteString(body)
	return research.Request{
		Topic:   topic,
		Context: b.String(),
		URLs:    urls,
	}, true
}

func (s *mailService) SimulateEnabled() bool {
	return s != nil && s.cfg.Simulate
}

func (s *mailService) ConnectSimulated(ctx context.Context, orgID uuid.UUID) (*model.MailConnectionStatus, error) {
	if !s.SimulateEnabled() || !s.Configured() {
		return nil, ErrMailNotConfigured
	}
	enc, err := mail.Encrypt(s.key, []byte("sim-refresh"))
	if err != nil {
		return nil, err
	}
	agent, _ := s.EnsureMailAgent(ctx, orgID)
	now := time.Now()
	email := "sim@jobshout.local"
	if p, ok := s.gmail.(interface {
		Profile(context.Context, string) (string, error)
	}); ok {
		if got, perr := p.Profile(ctx, "sim-access"); perr == nil && got != "" {
			email = got
		}
	}
	conn := &model.MailConnection{
		OrgID:                orgID,
		GoogleEmail:          email,
		RefreshTokenEnc:      enc,
		Scopes:               mail.RequestedScopes(),
		Status:               model.MailConnConnected,
		ConnectedAt:          &now,
		NextSyncAt:           &now,
		WatchLabels:          []string{},
		WatchSenders:         []string{},
		WatchSubjectPrefixes: []string{},
		WatchKnowledgeURLs:   []string{},
	}
	if agent != nil {
		conn.AgentID = &agent.ID
	}
	if existing, _ := s.repo.GetConnectionByOrg(ctx, orgID); existing != nil {
		conn.AllowMailboxMutations = existing.AllowMailboxMutations
		conn.WatchLabels = existing.WatchLabels
		conn.WatchSenders = existing.WatchSenders
		conn.WatchSubjectPrefixes = existing.WatchSubjectPrefixes
		conn.WatchKnowledgeURLs = existing.WatchKnowledgeURLs
		conn.ResearchFocus = existing.ResearchFocus
		conn.ReplyInstructions = existing.ReplyInstructions
	}
	if err := s.repo.UpsertConnection(ctx, conn); err != nil {
		return nil, err
	}
	if clearer, ok := s.gmail.(interface{ ClearInbox() }); ok {
		clearer.ClearInbox()
	}
	s.logger.Info("mail: simulated mailbox connected", zap.String("org_id", orgID.String()))
	return s.ConnectionStatus(ctx, orgID)
}

func (s *mailService) PushSimulatedInbox(msgs []mail.InboxMessage) error {
	if !s.SimulateEnabled() {
		return ErrMailNotConfigured
	}
	pusher, ok := s.gmail.(interface {
		Push(msgs ...mail.InboxMessage)
	})
	if !ok {
		return fmt.Errorf("mail: simulated inbox is not wired")
	}
	if clearer, ok := s.gmail.(interface{ ClearInbox() }); ok {
		clearer.ClearInbox()
	}
	pusher.Push(msgs...)
	return nil
}
