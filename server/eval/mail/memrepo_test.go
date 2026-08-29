package mail_eval

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// memMailRepo is an in-memory repository.MailRepository.
//
// It is deliberately local to the eval tree rather than shared with the service
// package's own fake: an evaluation should exercise the production interface
// from the outside, and coupling it to another package's test internals would
// make the suite fail for reasons that have nothing to do with the agent.
type memMailRepo struct {
	mu      sync.Mutex
	conns   map[uuid.UUID]*model.MailConnection
	threads map[uuid.UUID]*model.MailThread
	drafts  map[uuid.UUID]*model.MailDraft
	states  map[string]oauthState
}

type oauthState struct {
	orgID, userID uuid.UUID
	expires       time.Time
}

func newMemMailRepo() *memMailRepo {
	return &memMailRepo{
		conns:   map[uuid.UUID]*model.MailConnection{},
		threads: map[uuid.UUID]*model.MailThread{},
		drafts:  map[uuid.UUID]*model.MailDraft{},
		states:  map[string]oauthState{},
	}
}

func (r *memMailRepo) UpsertConnection(_ context.Context, c *model.MailConnection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	cp := *c
	r.conns[c.OrgID] = &cp
	return nil
}

func (r *memMailRepo) GetConnectionByOrg(_ context.Context, orgID uuid.UUID) (*model.MailConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.conns[orgID]
	if !ok {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (r *memMailRepo) Disconnect(_ context.Context, orgID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, orgID)
	return nil
}

func (r *memMailRepo) UpdateConnectionMeta(_ context.Context, c *model.MailConnection) error {
	return r.UpsertConnection(context.Background(), c)
}

func (r *memMailRepo) PutOAuthState(_ context.Context, state string, orgID, userID uuid.UUID, expires time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[state] = oauthState{orgID: orgID, userID: userID, expires: expires}
	return nil
}

func (r *memMailRepo) ConsumeOAuthState(_ context.Context, state string) (uuid.UUID, uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.states[state]
	if !ok {
		return uuid.Nil, uuid.Nil, context.Canceled
	}
	delete(r.states, state)
	return s.orgID, s.userID, nil
}

func (r *memMailRepo) UpsertThread(_ context.Context, t *model.MailThread) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	cp := *t
	r.threads[t.ID] = &cp
	return nil
}

func (r *memMailRepo) GetThread(_ context.Context, id uuid.UUID) (*model.MailThread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (r *memMailRepo) GetThreadByGmailID(_ context.Context, orgID uuid.UUID, gmailThreadID string) (*model.MailThread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.threads {
		if t.OrgID == orgID && t.GmailThreadID == gmailThreadID {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memMailRepo) ListThreads(_ context.Context, orgID uuid.UUID, _ model.PaginationParams) (*model.PaginatedResponse[model.MailThread], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.MailThread
	for _, t := range r.threads {
		if t.OrgID == orgID {
			out = append(out, *t)
		}
	}
	return &model.PaginatedResponse[model.MailThread]{Data: out}, nil
}

func (r *memMailRepo) UpdateThread(_ context.Context, t *model.MailThread) error {
	return r.UpsertThread(context.Background(), t)
}

func (r *memMailRepo) UpsertDraft(_ context.Context, d *model.MailDraft) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	cp := *d
	r.drafts[d.ID] = &cp
	return nil
}

func (r *memMailRepo) GetDraft(_ context.Context, id uuid.UUID) (*model.MailDraft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.drafts[id]
	if !ok {
		return nil, nil
	}
	cp := *d
	return &cp, nil
}

func (r *memMailRepo) GetDraftByThread(_ context.Context, threadID uuid.UUID) (*model.MailDraft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.drafts {
		if d.ThreadID == threadID {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memMailRepo) ListDraftsByStatus(_ context.Context, orgID uuid.UUID, status string, _ model.PaginationParams) (*model.PaginatedResponse[model.MailDraft], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.MailDraft
	for _, d := range r.drafts {
		if d.OrgID == orgID && d.Status == status {
			out = append(out, *d)
		}
	}
	return &model.PaginatedResponse[model.MailDraft]{Data: out}, nil
}

func (r *memMailRepo) UpdateDraft(_ context.Context, d *model.MailDraft) error {
	return r.UpsertDraft(context.Background(), d)
}

func (r *memMailRepo) ClaimDueConnections(_ context.Context, limit int, _ time.Duration) ([]model.MailConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.MailConnection
	for _, c := range r.conns {
		if len(out) >= limit {
			break
		}
		out = append(out, *c)
	}
	return out, nil
}

func (r *memMailRepo) MarkSynced(_ context.Context, id uuid.UUID, lastSync, nextSync time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		if c.ID == id {
			c.LastSyncAt = &lastSync
			c.NextSyncAt = &nextSync
		}
	}
	return nil
}

// --- read helpers used by the suite --------------------------------------

func (r *memMailRepo) threadsOf(orgID uuid.UUID) []model.MailThread {
	res, _ := r.ListThreads(context.Background(), orgID, model.PaginationParams{})
	return res.Data
}

func (r *memMailRepo) allDrafts() []model.MailDraft {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.MailDraft, 0, len(r.drafts))
	for _, d := range r.drafts {
		out = append(out, *d)
	}
	return out
}
