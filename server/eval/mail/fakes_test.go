package mail_eval

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/service"
)

// --- Gmail ---------------------------------------------------------------

// fakeGmail serves fixture messages and records every send.
//
// Send is the one call that must never happen without an explicit approval, so
// the recorder is the point of the whole fake.
type fakeGmail struct {
	mu       sync.Mutex
	messages []mail.InboxMessage
	sends    []mail.OutboundMessage
	listErr  error
	queries  []string
}

func (f *fakeGmail) ExchangeCode(context.Context, string, string, string, string) (mail.TokenSet, error) {
	return mail.TokenSet{AccessToken: "at", RefreshToken: "rt"}, nil
}

func (f *fakeGmail) Refresh(context.Context, string, string, string) (mail.TokenSet, error) {
	return mail.TokenSet{AccessToken: "access-token", Expiry: time.Now().Add(time.Hour)}, nil
}

func (f *fakeGmail) Profile(context.Context, string) (string, error) { return "ops@ourco.example", nil }

func (f *fakeGmail) ListMessages(_ context.Context, _, query string, _ int) ([]mail.InboxMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.messages, nil
}

func (f *fakeGmail) Send(_ context.Context, _ string, msg mail.OutboundMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, msg)
	return "sent-" + uuid.NewString(), nil
}

func (f *fakeGmail) SendCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

// --- Research ------------------------------------------------------------

// fakeResearch records the request it was handed and returns a canned brief.
//
// Recording research.Request whole — not just the topic — is deliberate: the
// interesting question is whether the sender's links reach the URLs field,
// which is what selects the agent's direct-fetch path.
type fakeResearch struct {
	mu        sync.Mutex
	requests  []research.Request
	brief     *research.Brief
	err       error
	available bool
	runID     uuid.UUID
}

func (f *fakeResearch) Research(_ context.Context, _ uuid.UUID, req research.Request, _ research.ProgressFunc) (*research.Brief, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return f.brief, nil
}

func (f *fakeResearch) Trending(context.Context, int) ([]research.TrendingItem, error) {
	return nil, nil
}

func (f *fakeResearch) Discover(context.Context, uuid.UUID, research.DiscoverRequest, research.ProgressFunc) ([]research.Topic, error) {
	return nil, nil
}

func (f *fakeResearch) EnsureResearcher(_ context.Context, orgID uuid.UUID) (*model.Agent, error) {
	return &model.Agent{ID: uuid.New(), OrgID: orgID, Name: "Research Agent"}, nil
}

func (f *fakeResearch) Available() bool { return f.available }

// Run is the persistence-aware entry point the Mail Agent uses so it can store
// the real research run id on the thread. The fake returns a run row with a
// stable id, which is what lets the suite assert the id the mail pipeline
// stored is the one research handed back rather than an invented UUID.
func (f *fakeResearch) Run(ctx context.Context, orgID uuid.UUID, req research.Request, progress research.ProgressFunc, _ service.ResearchRunOptions) (*service.ResearchOutcome, error) {
	brief, err := f.Research(ctx, orgID, req, progress)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	if f.runID == uuid.Nil {
		f.runID = uuid.New()
	}
	id := f.runID
	f.mu.Unlock()
	return &service.ResearchOutcome{
		Run:   &model.ResearchRun{ID: id, OrgID: orgID, Topic: req.Topic, Status: model.ResearchRunCompleted},
		Brief: brief,
	}, nil
}

// StartAsync is not exercised by the mail suite — the Mail Agent waits for its
// brief before drafting — but the fake must satisfy the interface.
func (f *fakeResearch) StartAsync(context.Context, uuid.UUID, research.Request, service.ResearchRunOptions) (*model.ResearchRun, error) {
	return nil, nil
}

func (f *fakeResearch) GetRun(context.Context, uuid.UUID, uuid.UUID) (*model.ResearchRun, error) {
	return nil, nil
}

func (f *fakeResearch) ListRuns(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.ResearchRun], error) {
	return &model.PaginatedResponse[model.ResearchRun]{}, nil
}

// RunID is the id the fake handed back, so a check can compare it with what the
// mail pipeline persisted.
func (f *fakeResearch) RunID() uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runID
}

func (f *fakeResearch) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeResearch) Last() (research.Request, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return research.Request{}, false
	}
	return f.requests[len(f.requests)-1], true
}

// --- Agent repository ----------------------------------------------------

type memAgentRepo struct {
	mu     sync.Mutex
	agents map[uuid.UUID]*model.Agent
}

func newMemAgentRepo() *memAgentRepo {
	return &memAgentRepo{agents: map[uuid.UUID]*model.Agent{}}
}

func (r *memAgentRepo) Create(_ context.Context, a *model.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	cp := *a
	r.agents[a.ID] = &cp
	return nil
}

func (r *memAgentRepo) FindByID(_ context.Context, id uuid.UUID) (*model.Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.agents[id]
	if !ok {
		return nil, errors.New("agent not found")
	}
	cp := *a
	return &cp, nil
}

func (r *memAgentRepo) FindBuiltin(_ context.Context, orgID uuid.UUID, builtin string) (*model.Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.agents {
		if a.OrgID == orgID && a.IsBuiltin(builtin) {
			cp := *a
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memAgentRepo) ListByOrg(_ context.Context, orgID uuid.UUID, _ model.PaginationParams, _ repository.AgentListFilter) (*model.PaginatedResponse[model.Agent], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.Agent
	for _, a := range r.agents {
		if a.OrgID == orgID {
			out = append(out, *a)
		}
	}
	return &model.PaginatedResponse[model.Agent]{Data: out}, nil
}

func (r *memAgentRepo) Update(_ context.Context, a *model.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *a
	r.agents[a.ID] = &cp
	return nil
}

func (r *memAgentRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, id)
	return nil
}

func (r *memAgentRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.agents[id]; ok {
		a.Status = status
	}
	return nil
}
