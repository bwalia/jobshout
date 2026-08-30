package taskmanager_eval

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
)

// recordingRunner stands in for a specialist pipeline and records what it was
// asked to do, so a check can assert the inputs that reached it rather than
// what the caller believed it sent.
type recordingRunner struct {
	mu         sync.Mutex
	builtin    string
	kind       string
	externalID string
	err        error
	calls      int
	lastInputs map[string]string
	lastRun    *model.AgentRun
}

func (r *recordingRunner) Builtin() string { return r.builtin }
func (r *recordingRunner) Kind() string    { return r.kind }

func (r *recordingRunner) Start(_ context.Context, run *model.AgentRun, _ *model.Agent, in map[string]string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.lastRun = run
	r.lastInputs = map[string]string{}
	for k, v := range in {
		r.lastInputs[k] = v
	}
	if r.err != nil {
		return "", r.err
	}
	return r.externalID, nil
}

func (r *recordingRunner) snapshot() (int, map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for k, v := range r.lastInputs {
		out[k] = v
	}
	return r.calls, out
}

// --- agent run repository ------------------------------------------------

type memAgentRunRepo struct {
	mu   sync.Mutex
	runs map[uuid.UUID]*model.AgentRun
	seq  []uuid.UUID
}

func newMemAgentRunRepo() *memAgentRunRepo {
	return &memAgentRunRepo{runs: map[uuid.UUID]*model.AgentRun{}}
}

func (r *memAgentRunRepo) Create(_ context.Context, run *model.AgentRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	cp := *run
	r.runs[run.ID] = &cp
	r.seq = append(r.seq, run.ID)
	return nil
}

func (r *memAgentRunRepo) GetByID(_ context.Context, id uuid.UUID) (*model.AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok {
		return nil, nil
	}
	cp := *run
	return &cp, nil
}

func (r *memAgentRunRepo) Update(_ context.Context, run *model.AgentRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runs[run.ID]; !ok {
		return errors.New("no such run")
	}
	cp := *run
	r.runs[run.ID] = &cp
	return nil
}

func (r *memAgentRunRepo) ListByOrg(_ context.Context, orgID uuid.UUID, _ model.PaginationParams) (*model.PaginatedResponse[model.AgentRun], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.AgentRun
	for _, id := range r.seq {
		if run := r.runs[id]; run.OrgID == orgID {
			out = append(out, *run)
		}
	}
	return &model.PaginatedResponse[model.AgentRun]{Data: out, Total: len(out)}, nil
}

func (r *memAgentRunRepo) all() []model.AgentRun {
	res, _ := r.ListByOrg(context.Background(), r.anyOrg(), model.PaginationParams{})
	return res.Data
}

func (r *memAgentRunRepo) anyOrg() uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range r.seq {
		return r.runs[id].OrgID
	}
	return uuid.Nil
}

func (r *memAgentRunRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

// --- agent repository ----------------------------------------------------

type memAgentRepo struct {
	mu     sync.Mutex
	agents map[uuid.UUID]*model.Agent
}

func newMemAgentRepo(agents ...model.Agent) *memAgentRepo {
	r := &memAgentRepo{agents: map[uuid.UUID]*model.Agent{}}
	for i := range agents {
		cp := agents[i]
		r.agents[cp.ID] = &cp
	}
	return r
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
	return &model.PaginatedResponse[model.Agent]{Data: out, Total: len(out)}, nil
}

func (r *memAgentRepo) Update(context.Context, *model.Agent) error            { return nil }
func (r *memAgentRepo) Delete(context.Context, uuid.UUID) error               { return nil }
func (r *memAgentRepo) UpdateStatus(context.Context, uuid.UUID, string) error { return nil }

// --- agent service, for the chat path ------------------------------------

// stubAgentService is the slice of service.AgentService the chat tools use.
type stubAgentService struct{ repo *memAgentRepo }

func (s *stubAgentService) Create(context.Context, uuid.UUID, uuid.UUID, model.CreateAgentRequest) (*model.Agent, error) {
	return nil, nil
}

func (s *stubAgentService) GetByID(ctx context.Context, id uuid.UUID) (*model.Agent, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *stubAgentService) List(ctx context.Context, orgID uuid.UUID, p model.PaginationParams, f repository.AgentListFilter) (*model.PaginatedResponse[model.Agent], error) {
	return s.repo.ListByOrg(ctx, orgID, p, f)
}

func (s *stubAgentService) Update(context.Context, uuid.UUID, model.UpdateAgentRequest) (*model.Agent, error) {
	return nil, nil
}
func (s *stubAgentService) Delete(context.Context, uuid.UUID) error               { return nil }
func (s *stubAgentService) UpdateStatus(context.Context, uuid.UUID, string) error { return nil }

var _ service.AgentService = (*stubAgentService)(nil)

// The specialist services below are embedded interfaces with only the methods
// a start tool is *allowed* to call. Anything else panics on a nil interface —
// which is the point. These stubs are the executable form of "a specialist tool
// may check whether it is configured, then hand off; it may not start work
// itself".
type stubResearchSvc struct{ service.ResearchService }

func (stubResearchSvc) Available() bool { return true }

type stubBlogSvc struct{ service.BlogService }

type stubPentestSvc struct{ service.PentestService }

type stubMailSvc struct{ service.MailService }

func (stubMailSvc) Available(context.Context, uuid.UUID) bool { return true }
