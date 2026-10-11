package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// memRepo is an in-memory WebhookRepository.
type memRepo struct {
	mu         sync.Mutex
	endpoints  map[uuid.UUID]model.WebhookEndpoint
	deliveries map[uuid.UUID]*model.WebhookDelivery
	now        func() time.Time
}

func newMemRepo(now func() time.Time) *memRepo {
	return &memRepo{endpoints: map[uuid.UUID]model.WebhookEndpoint{}, deliveries: map[uuid.UUID]*model.WebhookDelivery{}, now: now}
}

func (m *memRepo) CreateEndpoint(_ context.Context, e *model.WebhookEndpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	m.endpoints[e.ID] = *e
	return nil
}
func (m *memRepo) GetEndpoint(_ context.Context, orgID, id uuid.UUID) (*model.WebhookEndpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.endpoints[id]
	if !ok || e.OrgID != orgID {
		return nil, repository.ErrWebhookNotFound
	}
	return &e, nil
}
func (m *memRepo) GetEndpointForDelivery(_ context.Context, id uuid.UUID) (*model.WebhookEndpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.endpoints[id]
	if !ok {
		return nil, repository.ErrWebhookNotFound
	}
	return &e, nil
}
func (m *memRepo) ListEndpoints(context.Context, uuid.UUID) ([]model.WebhookEndpoint, error) {
	return nil, nil
}
func (m *memRepo) UpdateEndpoint(context.Context, *model.WebhookEndpoint) error { return nil }
func (m *memRepo) DeleteEndpoint(context.Context, uuid.UUID, uuid.UUID) error   { return nil }
func (m *memRepo) ListSubscribed(_ context.Context, orgID uuid.UUID, ev string) ([]model.WebhookEndpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.WebhookEndpoint
	for _, e := range m.endpoints {
		if e.OrgID == orgID && e.Enabled && e.Subscribed(ev) {
			out = append(out, e)
		}
	}
	return out, nil
}
func (m *memRepo) CreateDelivery(_ context.Context, d *model.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d.ID = uuid.New()
	cp := *d
	m.deliveries[d.ID] = &cp
	return nil
}
func (m *memRepo) ClaimDue(_ context.Context, limit int, lease time.Duration) ([]model.WebhookDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.WebhookDelivery
	for _, d := range m.deliveries {
		if d.Status == model.WebhookDeliveryPending && d.NextAttemptAt != nil && !d.NextAttemptAt.After(m.now()) {
			t := m.now().Add(lease)
			d.NextAttemptAt = &t
			out = append(out, *d)
		}
	}
	return out, nil
}
func (m *memRepo) RecordAttempt(_ context.Context, id uuid.UUID, status string, attempts int, code *int, errMsg *string, next *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.deliveries[id]
	d.Status, d.Attempts, d.LastStatusCode, d.LastError, d.NextAttemptAt = status, attempts, code, errMsg, next
	return nil
}
func (m *memRepo) ListDeliveries(context.Context, uuid.UUID, uuid.UUID, int) ([]model.WebhookDelivery, error) {
	return nil, nil
}
func (m *memRepo) only(t *testing.T) *model.WebhookDelivery {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(m.deliveries))
	}
	for _, d := range m.deliveries {
		cp := *d
		return &cp
	}
	return nil
}

type receiver struct {
	mu     sync.Mutex
	status int
	got    []*http.Request
	bodies [][]byte
}

func (rc *receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	rc.mu.Lock()
	rc.got, rc.bodies = append(rc.got, r), append(rc.bodies, b)
	st := rc.status
	rc.mu.Unlock()
	w.WriteHeader(st)
}

func TestEmit_SignedDeliveryToSubscribedEndpointsOfTheOrgOnly(t *testing.T) {
	rc := &receiver{status: 200}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	repo := newMemRepo(time.Now)
	orgA, orgB := uuid.New(), uuid.New()
	secret := "whsec_test_secret_value_123"
	_ = repo.CreateEndpoint(context.Background(), &model.WebhookEndpoint{OrgID: orgA, URL: srv.URL, Secret: secret, Events: []string{model.WebhookEventRunCompleted}, Enabled: true})
	_ = repo.CreateEndpoint(context.Background(), &model.WebhookEndpoint{OrgID: orgA, URL: srv.URL, Secret: secret, Events: []string{model.WebhookEventApprovalRequested}, Enabled: true})
	_ = repo.CreateEndpoint(context.Background(), &model.WebhookEndpoint{OrgID: orgB, URL: srv.URL, Secret: "other-org-secret-xxxx", Events: []string{"*"}, Enabled: true})
	d := NewDispatcher(repo, Options{Sync: true})

	d.Emit(context.Background(), orgA, model.WebhookEventRunCompleted, map[string]string{"run_id": "r1"})

	if len(rc.got) != 1 {
		t.Fatalf("receiver got %d requests, want 1 (only org A's run.completed endpoint)", len(rc.got))
	}
	req, body := rc.got[0], rc.bodies[0]
	ts, _ := strconv.ParseInt(req.Header.Get(HeaderTimestamp), 10, 64)
	if !Verify(secret, ts, body, req.Header.Get(HeaderSignature)) {
		t.Fatal("signature does not verify with the endpoint secret")
	}
	if Verify("other-org-secret-xxxx", ts, body, req.Header.Get(HeaderSignature)) {
		t.Fatal("signature verifies with another secret")
	}
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil || env.Type != model.WebhookEventRunCompleted || env.OrgID != orgA {
		t.Fatalf("envelope = %+v, %v", env, err)
	}
	if req.Header.Get(HeaderEvent) != model.WebhookEventRunCompleted || req.Header.Get(HeaderEventID) != env.ID.String() {
		t.Fatal("event headers missing")
	}
	if del := repo.only(t); del.Status != model.WebhookDeliverySucceeded || del.Attempts != 1 {
		t.Fatalf("delivery = %+v", del)
	}
}

func TestRetryWithBackoffThenFail(t *testing.T) {
	rc := &receiver{status: 503}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	now := time.Unix(1_700_000_000, 0)
	clock := func() time.Time { return now }
	repo := newMemRepo(clock)
	org := uuid.New()
	_ = repo.CreateEndpoint(context.Background(), &model.WebhookEndpoint{OrgID: org, URL: srv.URL, Secret: "s", Events: []string{"*"}, Enabled: true})
	d := NewDispatcher(repo, Options{Sync: true, Backoff: []time.Duration{time.Minute, 5 * time.Minute}})
	d.now = clock

	d.Emit(context.Background(), org, model.WebhookEventRunFailed, nil)
	del := repo.only(t)
	if del.Status != model.WebhookDeliveryPending || del.Attempts != 1 || *del.LastStatusCode != 503 ||
		!del.NextAttemptAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("after attempt 1: %+v next=%v", del, del.NextAttemptAt)
	}
	if n := d.RetryDue(context.Background()); n != 0 {
		t.Fatalf("retried %d before the backoff elapsed", n)
	}
	now = now.Add(time.Minute)
	d.RetryDue(context.Background())
	del = repo.only(t)
	if del.Attempts != 2 || !del.NextAttemptAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("after attempt 2: attempts=%d next=%v", del.Attempts, del.NextAttemptAt)
	}
	// Third attempt succeeds-or-fails for good (cap = len(backoff)+1 = 3).
	now = now.Add(5 * time.Minute)
	d.RetryDue(context.Background())
	del = repo.only(t)
	if del.Status != model.WebhookDeliveryFailed || del.Attempts != 3 || del.NextAttemptAt != nil {
		t.Fatalf("after cap: %+v", del)
	}
	if len(rc.got) != 3 {
		t.Fatalf("receiver got %d attempts, want 3", len(rc.got))
	}
	// Retries carry the same event id.
	if rc.got[0].Header.Get(HeaderEventID) != rc.got[2].Header.Get(HeaderEventID) {
		t.Fatal("event id changed across retries")
	}

	// A recovered receiver succeeds on retry.
	rc.status = 200
	repo2 := newMemRepo(clock)
	_ = repo2.CreateEndpoint(context.Background(), &model.WebhookEndpoint{OrgID: org, URL: srv.URL, Secret: "s", Events: []string{"*"}, Enabled: true})
	d2 := NewDispatcher(repo2, Options{Sync: true, Backoff: []time.Duration{time.Minute}})
	d2.now = clock
	rc.status = 500
	d2.Emit(context.Background(), org, model.WebhookEventRunFailed, nil)
	rc.status = 200
	now = now.Add(time.Minute)
	d2.RetryDue(context.Background())
	if del := repo2.only(t); del.Status != model.WebhookDeliverySucceeded || del.Attempts != 2 {
		t.Fatalf("recovered delivery = %+v", del)
	}
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

type recEmitter struct {
	events []string
	orgs   []uuid.UUID
	data   []any
}

func (r *recEmitter) Emit(_ context.Context, orgID uuid.UUID, ev string, data any) {
	r.events, r.orgs, r.data = append(r.events, ev), append(r.orgs, orgID), append(r.data, data)
}

type memRuns struct {
	repository.TaskRunRepository
	rows map[uuid.UUID]model.TaskRun
}

func (m *memRuns) GetByID(_ context.Context, id uuid.UUID) (*model.TaskRun, error) {
	r, ok := m.rows[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &r, nil
}
func (m *memRuns) Update(_ context.Context, run *model.TaskRun) error {
	m.rows[run.ID] = *run
	return nil
}

func TestTaskRunsHook_EmitsOncePerTerminalTransition(t *testing.T) {
	em := &recEmitter{}
	org := uuid.New()
	run := model.TaskRun{ID: uuid.New(), OrgID: org, Status: model.TaskRunStatusQueued}
	inner := &memRuns{rows: map[uuid.UUID]model.TaskRun{run.ID: run}}
	h := &TaskRuns{TaskRunRepository: inner, Emitter: em}
	ctx := context.Background()

	run.Status = model.TaskRunStatusRunning
	_ = h.Update(ctx, &run)
	out := string(make([]byte, maxOutput+10))
	run.Status, run.Output = model.TaskRunStatusCompleted, &out
	_ = h.Update(ctx, &run)
	_ = h.Update(ctx, &run) // same terminal status again: no second event
	if len(em.events) != 1 || em.events[0] != model.WebhookEventRunCompleted || em.orgs[0] != org {
		t.Fatalf("events = %v", em.events)
	}
	if d := em.data[0].(RunData); !d.OutputTruncated || len(*d.Output) != maxOutput {
		t.Fatal("large output not truncated")
	}

	run2 := model.TaskRun{ID: uuid.New(), OrgID: org, Status: model.TaskRunStatusRunning}
	inner.rows[run2.ID] = run2
	run2.Status = model.TaskRunStatusFailed
	_ = h.Update(ctx, &run2)
	if len(em.events) != 2 || em.events[1] != model.WebhookEventRunFailed {
		t.Fatalf("events = %v", em.events)
	}
}

type oneAgent struct{ a *model.Agent }

func (o oneAgent) FindByID(context.Context, uuid.UUID) (*model.Agent, error) { return o.a, nil }

func TestTaskRunsHook_CarriesPromptVersionAndModel(t *testing.T) {
	em := &recEmitter{}
	org, agentID := uuid.New(), uuid.New()
	provider, name := "openai", "qwen3"
	run := model.TaskRun{ID: uuid.New(), OrgID: org, AgentID: agentID, Status: model.TaskRunStatusRunning,
		ModelProvider: &provider, ModelName: &name}
	inner := &memRuns{rows: map[uuid.UUID]model.TaskRun{run.ID: run}}
	agent := &model.Agent{ID: agentID, OrgID: org, Metadata: map[string]any{"prompt_version": "seller-pa@1.0.0"}}
	h := &TaskRuns{TaskRunRepository: inner, Emitter: em, Agents: oneAgent{agent}}

	run.Status = model.TaskRunStatusCompleted
	_ = h.Update(context.Background(), &run)
	d := em.data[0].(RunData)
	if d.PromptVersion != "seller-pa@1.0.0" || d.ModelName == nil || *d.ModelName != "qwen3" {
		t.Fatalf("run data = %+v", d)
	}

	// An agent of another org never lends its metadata.
	agent.OrgID = uuid.New()
	run2 := model.TaskRun{ID: uuid.New(), OrgID: org, AgentID: agentID, Status: model.TaskRunStatusRunning}
	inner.rows[run2.ID] = run2
	run2.Status = model.TaskRunStatusFailed
	_ = h.Update(context.Background(), &run2)
	if d := em.data[1].(RunData); d.PromptVersion != "" {
		t.Fatalf("cross-org prompt version leaked: %q", d.PromptVersion)
	}
}

type memApprovals struct {
	repository.ApprovalRepository
	rows map[uuid.UUID]model.Approval
}

func (m *memApprovals) Create(_ context.Context, a *model.Approval) error {
	m.rows[a.ID] = *a
	return nil
}
func (m *memApprovals) UpdateDecision(_ context.Context, id uuid.UUID, status, _ string, by uuid.UUID) error {
	a, ok := m.rows[id]
	if !ok || a.Status != model.ApprovalStatusPending {
		return errors.New("not pending")
	}
	a.Status, a.DecidedBy = status, &by
	m.rows[id] = a
	return nil
}
func (m *memApprovals) FindByID(_ context.Context, id uuid.UUID) (*model.Approval, error) {
	a := m.rows[id]
	return &a, nil
}

type runLookup struct{ run *model.TaskRun }

func (r runLookup) GetByExecutionID(context.Context, uuid.UUID) (*model.TaskRun, error) {
	return r.run, nil
}

func TestApprovalsHook(t *testing.T) {
	em := &recEmitter{}
	org := uuid.New()
	runID := uuid.New()
	h := &Approvals{ApprovalRepository: &memApprovals{rows: map[uuid.UUID]model.Approval{}}, Emitter: em,
		Runs: runLookup{run: &model.TaskRun{ID: runID}}}
	ap := &model.Approval{ID: uuid.New(), OrgID: org, ExecutionID: uuid.New(), ToolName: "send_offer", Status: model.ApprovalStatusPending}
	ctx := context.Background()
	if err := h.Create(ctx, ap); err != nil {
		t.Fatal(err)
	}
	if err := h.UpdateDecision(ctx, ap.ID, model.ApprovalStatusApproved, "ok", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := h.UpdateDecision(ctx, ap.ID, model.ApprovalStatusRejected, "", uuid.New()); err == nil {
		t.Fatal("second decision accepted")
	}
	if len(em.events) != 2 || em.events[0] != model.WebhookEventApprovalRequested || em.events[1] != model.WebhookEventApprovalDecided {
		t.Fatalf("events = %v (a failed decision must not emit)", em.events)
	}
	d := em.data[1].(ApprovalData)
	if d.Status != model.ApprovalStatusApproved || d.TaskRunID == nil || *d.TaskRunID != runID || em.orgs[1] != org {
		t.Fatalf("decided data = %+v", d)
	}
}
