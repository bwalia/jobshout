package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// memApprovals is an in-memory ApprovalRepository with the same
// decide/claim/resume semantics as the Postgres one.
type memApprovals struct {
	repository.ApprovalRepository
	mu      sync.Mutex
	rows    map[uuid.UUID]*model.Approval
	claimed map[uuid.UUID]time.Time
}

func newMemApprovals(rows ...*model.Approval) *memApprovals {
	m := &memApprovals{rows: map[uuid.UUID]*model.Approval{}, claimed: map[uuid.UUID]time.Time{}}
	for _, r := range rows {
		m.rows[r.ID] = r
	}
	return m
}

func (m *memApprovals) FindByID(_ context.Context, id uuid.UUID) (*model.Approval, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.rows[id]
	if !ok {
		return nil, repository.ErrApprovalNotFound
	}
	cp := *a
	return &cp, nil
}

func (m *memApprovals) UpdateDecision(_ context.Context, id uuid.UUID, status, reason string, by uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.rows[id]
	if !ok {
		return repository.ErrApprovalNotFound
	}
	if a.Status != model.ApprovalStatusPending {
		return repository.ErrApprovalNotPending
	}
	now := time.Now()
	a.Status, a.DecidedBy, a.DecidedAt = status, &by, &now
	if reason != "" {
		a.Reason = &reason
	}
	return nil
}

func (m *memApprovals) ClaimResume(_ context.Context, id uuid.UUID, staleAfter time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.rows[id]
	if a == nil || a.Status == model.ApprovalStatusPending || a.ResumedAt != nil {
		return false, nil
	}
	if at, ok := m.claimed[id]; ok && time.Since(at) <= staleAfter {
		return false, nil
	}
	m.claimed[id] = time.Now()
	return true, nil
}

func (m *memApprovals) MarkResumed(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.rows[id].ResumedAt = &now
	return nil
}

func (m *memApprovals) ListUnresumed(_ context.Context, staleAfter time.Duration, _ int) ([]model.Approval, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.Approval
	for id, a := range m.rows {
		if a.Status == model.ApprovalStatusPending || a.ResumedAt != nil {
			continue
		}
		if at, ok := m.claimed[id]; ok && time.Since(at) <= staleAfter {
			continue
		}
		out = append(out, *a)
	}
	return out, nil
}

// fakeResumeExecs is the execution side of a resume.
type fakeResumeExecs struct {
	ExecutionService
	mu        sync.Mutex
	paused    bool // whether MarkResuming finds the execution paused
	completed []executor.Result
}

func (f *fakeResumeExecs) MarkResuming(context.Context, uuid.UUID) (bool, error) {
	return f.paused, nil
}

func (f *fakeResumeExecs) GetByID(_ context.Context, id uuid.UUID) (*model.AgentExecution, error) {
	return &model.AgentExecution{ID: id, Status: model.ExecutionStatusRunning}, nil
}

func (f *fakeResumeExecs) CompleteResumed(_ context.Context, id uuid.UUID, res executor.Result) (*model.AgentExecution, error) {
	f.mu.Lock()
	f.completed = append(f.completed, res)
	f.mu.Unlock()
	out := &model.AgentExecution{ID: id, TotalTokens: res.TotalTokens, LatencyMs: res.LatencyMs}
	switch {
	case res.Status == executor.StatusAwaitingApproval:
		out.Status = model.ExecutionStatusAwaitingApproval
		next := res.ApprovalID
		out.PendingApprovalID = &next
	case res.Err != nil:
		msg := res.Err.Error()
		out.Status, out.ErrorMessage = model.ExecutionStatusFailed, &msg
	default:
		out.Status, out.Output = model.ExecutionStatusCompleted, &res.FinalAnswer
	}
	return out, nil
}

// fakeResumer scripts Resume; release (if set) blocks it until closed.
type fakeResumer struct {
	mu      sync.Mutex
	result  executor.Result
	release chan struct{}
	seen    []*model.Approval
	ctxErr  []error
	hadDL   []bool
}

func (r *fakeResumer) Resume(ctx context.Context, a *model.Approval) executor.Result {
	if r.release != nil {
		<-r.release
	}
	_, dl := ctx.Deadline()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, a)
	r.ctxErr = append(r.ctxErr, ctx.Err())
	r.hadDL = append(r.hadDL, dl)
	return r.result
}

type fakeRunSync struct {
	mu       sync.Mutex
	statuses []string
	pending  []*uuid.UUID
}

func (f *fakeRunSync) SyncFromExecution(_ context.Context, e *model.AgentExecution) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, e.Status)
	f.pending = append(f.pending, e.PendingApprovalID)
	return nil
}

func pendingApproval() *model.Approval {
	return &model.Approval{
		ID: uuid.New(), OrgID: uuid.New(), ExecutionID: uuid.New(), AgentID: uuid.New(),
		ToolName: "send_email", Status: model.ApprovalStatusPending, ResumeState: []byte(`{}`),
	}
}

func newTestApprovalService(repo *memApprovals, execs *fakeResumeExecs, res *fakeResumer, sync *fakeRunSync) *approvalService {
	svc := &approvalService{
		repo: repo, execSvc: execs, exec: res, logger: zap.NewNop(),
		resumeTimeout: time.Minute, staleAfter: 2 * time.Minute,
	}
	if sync != nil { // a typed nil would be a non-nil interface
		svc.runSync = sync
	}
	return svc
}

// TestApprovalDecide_ReturnsBeforeResumeAndWritesOutcome: Decide records the
// decision and returns while the resume is still blocked, the resume runs on
// its own (deadline-bound, uncancelled) context even after the caller's is
// cancelled, and its outcome reaches the execution and the task run.
func TestApprovalDecide_ReturnsBeforeResumeAndWritesOutcome(t *testing.T) {
	cases := []struct {
		name         string
		decision     string
		wantStatus   string
		result       executor.Result
		wantRunAfter string
	}{
		{"approve completes", "approve", model.ApprovalStatusApproved,
			executor.Result{FinalAnswer: "sent", TotalTokens: 42, LatencyMs: 900}, model.ExecutionStatusCompleted},
		{"reject finishes", "reject", model.ApprovalStatusRejected,
			executor.Result{FinalAnswer: "ok, not sending", TotalTokens: 30}, model.ExecutionStatusCompleted},
		{"resume fails", "approve", model.ApprovalStatusApproved,
			executor.Result{Err: errors.New("boom")}, model.ExecutionStatusFailed},
		{"pauses again", "approve", model.ApprovalStatusApproved,
			executor.Result{Status: executor.StatusAwaitingApproval, ApprovalID: uuid.New()}, model.ExecutionStatusAwaitingApproval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := pendingApproval()
			repo := newMemApprovals(a)
			execs := &fakeResumeExecs{paused: true}
			res := &fakeResumer{result: tc.result, release: make(chan struct{})}
			rs := &fakeRunSync{}
			svc := newTestApprovalService(repo, execs, res, rs)

			ctx, cancel := context.WithCancel(context.Background())
			got, err := svc.Decide(ctx, a.ID, uuid.New(), tc.decision, "because")
			cancel() // the request is over; the resume must not care
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if got.Status != tc.wantStatus {
				t.Fatalf("decided status %q, want %q", got.Status, tc.wantStatus)
			}
			execs.mu.Lock()
			if len(execs.completed) != 0 {
				t.Fatal("Decide must return before the resumed run finishes")
			}
			execs.mu.Unlock()

			close(res.release)
			svc.inflight.Wait()

			if len(res.seen) != 1 || res.seen[0].Status != tc.wantStatus {
				t.Fatalf("resumer saw %+v", res.seen)
			}
			if res.ctxErr[0] != nil || !res.hadDL[0] {
				t.Fatalf("resume must run on its own live, deadline-bound context (err=%v deadline=%v)", res.ctxErr[0], res.hadDL[0])
			}
			if len(execs.completed) != 1 {
				t.Fatalf("outcome must be persisted once, got %d", len(execs.completed))
			}
			if len(rs.statuses) != 2 || rs.statuses[0] != model.ExecutionStatusRunning || rs.statuses[1] != tc.wantRunAfter {
				t.Fatalf("task run sync: %v", rs.statuses)
			}
			if tc.wantRunAfter == model.ExecutionStatusAwaitingApproval &&
				(rs.pending[1] == nil || *rs.pending[1] != tc.result.ApprovalID) {
				t.Fatal("a run paused again must carry the next approval id")
			}
			stored, _ := repo.FindByID(context.Background(), a.ID)
			if stored.ResumedAt == nil {
				t.Fatal("approval must be marked resumed")
			}
		})
	}
}

func TestApprovalDecide_Idempotency(t *testing.T) {
	t.Run("second decide conflicts", func(t *testing.T) {
		a := pendingApproval()
		svc := newTestApprovalService(newMemApprovals(a), &fakeResumeExecs{paused: true}, &fakeResumer{}, nil)
		if _, err := svc.Decide(context.Background(), a.ID, uuid.New(), "approve", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Decide(context.Background(), a.ID, uuid.New(), "reject", ""); !errors.Is(err, ErrApprovalAlreadyDecided) {
			t.Fatalf("want ErrApprovalAlreadyDecided, got %v", err)
		}
		svc.inflight.Wait()
	})

	t.Run("concurrent decides: one wins, one resume", func(t *testing.T) {
		a := pendingApproval()
		execs := &fakeResumeExecs{paused: true}
		res := &fakeResumer{}
		svc := newTestApprovalService(newMemApprovals(a), execs, res, nil)
		var wg sync.WaitGroup
		errs := make([]error, 8)
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, errs[i] = svc.Decide(context.Background(), a.ID, uuid.New(), "approve", "")
			}(i)
		}
		wg.Wait()
		svc.inflight.Wait()
		wins := 0
		for _, err := range errs {
			switch {
			case err == nil:
				wins++
			case !errors.Is(err, ErrApprovalAlreadyDecided):
				t.Fatalf("unexpected error %v", err)
			}
		}
		if wins != 1 || len(res.seen) != 1 || len(execs.completed) != 1 {
			t.Fatalf("wins=%d resumes=%d persisted=%d", wins, len(res.seen), len(execs.completed))
		}
	})

	t.Run("unknown approval", func(t *testing.T) {
		svc := newTestApprovalService(newMemApprovals(), &fakeResumeExecs{}, &fakeResumer{}, nil)
		if _, err := svc.Decide(context.Background(), uuid.New(), uuid.New(), "approve", ""); !errors.Is(err, ErrApprovalNotFound) {
			t.Fatalf("want ErrApprovalNotFound, got %v", err)
		}
	})
}

// TestApprovalResume_ExecutionNoLongerPaused: a run cancelled while it waited is
// not resumed, and the approval is closed so recovery does not retry it.
func TestApprovalResume_ExecutionNoLongerPaused(t *testing.T) {
	a := pendingApproval()
	repo := newMemApprovals(a)
	execs := &fakeResumeExecs{paused: false}
	res := &fakeResumer{}
	svc := newTestApprovalService(repo, execs, res, &fakeRunSync{})
	if _, err := svc.Decide(context.Background(), a.ID, uuid.New(), "approve", ""); err != nil {
		t.Fatal(err)
	}
	svc.inflight.Wait()
	if len(res.seen) != 0 || len(execs.completed) != 0 {
		t.Fatal("a run that is no longer paused must not be resumed")
	}
	if stored, _ := repo.FindByID(context.Background(), a.ID); stored.ResumedAt == nil {
		t.Fatal("approval must be closed so recovery does not retry it")
	}
}

// TestApprovalRecovery_ResumesUnfinished: a decided approval whose resume never
// ran (server restarted) is resumed by recovery, exactly once.
func TestApprovalRecovery_ResumesUnfinished(t *testing.T) {
	a := pendingApproval()
	decidedBy := uuid.New()
	a.Status, a.DecidedBy = model.ApprovalStatusApproved, &decidedBy
	untouched := pendingApproval() // still pending: not for recovery
	repo := newMemApprovals(a, untouched)
	execs := &fakeResumeExecs{paused: true}
	res := &fakeResumer{result: executor.Result{FinalAnswer: "done"}}
	svc := newTestApprovalService(repo, execs, res, &fakeRunSync{})

	svc.RunRecovery(context.Background(), 0)
	svc.inflight.Wait()
	svc.RunRecovery(context.Background(), 0) // a second pass finds nothing left
	svc.inflight.Wait()

	if len(res.seen) != 1 || res.seen[0].ID != a.ID {
		t.Fatalf("recovery should resume only the decided approval, once: %+v", res.seen)
	}
}
