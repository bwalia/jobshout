package research_eval

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
)

// --- research backend ----------------------------------------------------

// fixedBackend serves canned search results and documents, and records what was
// searched and fetched.
//
// Recording the fetches is the point for the pinned-URL case: the difference
// between "read this page" and "search the web about this subject" is visible
// only in which calls were made.
type fixedBackend struct {
	mu        sync.Mutex
	sources   []research.Source
	docs      map[string]*research.Document
	fetchErrs map[string]error
	searchN   int
	fetched   []string
}

func (f *fixedBackend) Name() string { return "fixed" }

func (f *fixedBackend) Search(context.Context, string, int) ([]research.Source, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchN++
	return f.sources, nil
}

func (f *fixedBackend) Fetch(_ context.Context, rawURL string) (*research.Document, error) {
	f.mu.Lock()
	f.fetched = append(f.fetched, rawURL)
	errFor, hasErr := f.fetchErrs[rawURL]
	doc, hasDoc := f.docs[rawURL]
	f.mu.Unlock()

	if hasErr {
		return nil, errFor
	}
	if hasDoc {
		return doc, nil
	}
	return nil, fmt.Errorf("no such document: %s", rawURL)
}

func (f *fixedBackend) searchCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.searchN
}

func (f *fixedBackend) fetchedURLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.fetched))
	copy(out, f.fetched)
	return out
}

// --- run repository ------------------------------------------------------

// memRunRepo is an in-memory repository.ResearchRunRepository. It records every
// status the run passed through, so a check can assert the lifecycle rather
// than only the final state.
type memRunRepo struct {
	mu       sync.Mutex
	runs     map[uuid.UUID]*model.ResearchRun
	statuses []string
	phases   []string
	failNext error
}

func newMemRunRepo() *memRunRepo {
	return &memRunRepo{runs: map[uuid.UUID]*model.ResearchRun{}}
}

func (r *memRunRepo) Create(_ context.Context, run *model.ResearchRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failNext != nil {
		err := r.failNext
		r.failNext = nil
		return err
	}
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	cp := *run
	r.runs[run.ID] = &cp
	r.statuses = append(r.statuses, run.Status)
	return nil
}

func (r *memRunRepo) GetByID(_ context.Context, id uuid.UUID) (*model.ResearchRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok {
		return nil, nil
	}
	cp := *run
	return &cp, nil
}

func (r *memRunRepo) Update(_ context.Context, run *model.ResearchRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runs[run.ID]; !ok {
		return errors.New("no such run")
	}
	cp := *run
	r.runs[run.ID] = &cp
	r.statuses = append(r.statuses, run.Status)
	return nil
}

func (r *memRunRepo) UpdatePhase(_ context.Context, id uuid.UUID, phase string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok {
		return errors.New("no such run")
	}
	run.Phase = phase
	r.phases = append(r.phases, phase)
	return nil
}

func (r *memRunRepo) ListByOrg(_ context.Context, orgID uuid.UUID, p model.PaginationParams) (*model.PaginatedResponse[model.ResearchRun], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []model.ResearchRun
	for _, run := range r.runs {
		if run.OrgID == orgID {
			out = append(out, *run)
		}
	}
	return &model.PaginatedResponse[model.ResearchRun]{Data: out, Total: len(out)}, nil
}

func (r *memRunRepo) only() *model.ResearchRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.runs {
		cp := *run
		return &cp
	}
	return nil
}

func (r *memRunRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

func (r *memRunRepo) phaseTrail() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.phases))
	copy(out, r.phases)
	return out
}

// --- agent repository ----------------------------------------------------

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
	return &model.PaginatedResponse[model.Agent]{}, nil
}

func (r *memAgentRepo) Update(context.Context, *model.Agent) error            { return nil }
func (r *memAgentRepo) Delete(context.Context, uuid.UUID) error               { return nil }
func (r *memAgentRepo) UpdateStatus(context.Context, uuid.UUID, string) error { return nil }
