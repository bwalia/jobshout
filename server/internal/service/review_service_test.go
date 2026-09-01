package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/reviewbot"
)

type mockReviewRunRepository struct {
	runs map[uuid.UUID]*model.ReviewRun
}

func newMockReviewRunRepository() *mockReviewRunRepository {
	return &mockReviewRunRepository{runs: make(map[uuid.UUID]*model.ReviewRun)}
}

func (m *mockReviewRunRepository) Create(_ context.Context, run *model.ReviewRun) error {
	clone := *run
	if clone.StageLog != nil {
		clone.StageLog = append([]string(nil), clone.StageLog...)
	}
	m.runs[run.ID] = &clone
	return nil
}

func (m *mockReviewRunRepository) GetByID(_ context.Context, id uuid.UUID) (*model.ReviewRun, error) {
	run, ok := m.runs[id]
	if !ok {
		return nil, errors.New("review run not found")
	}
	clone := *run
	return &clone, nil
}

func (m *mockReviewRunRepository) Update(_ context.Context, run *model.ReviewRun) error {
	clone := *run
	if clone.StageLog != nil {
		clone.StageLog = append([]string(nil), clone.StageLog...)
	}
	m.runs[run.ID] = &clone
	return nil
}

func (m *mockReviewRunRepository) ListByOrg(_ context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.ReviewRun], error) {
	var data []model.ReviewRun
	for _, run := range m.runs {
		if run.OrgID == orgID {
			data = append(data, *run)
		}
	}
	return &model.PaginatedResponse[model.ReviewRun]{
		Data: data, Total: len(data), Page: pagination.Page, PerPage: pagination.PerPage, TotalPages: 1,
	}, nil
}

func (m *mockReviewRunRepository) FindActive(_ context.Context, orgID uuid.UUID, repo string, prNumber int) (*model.ReviewRun, error) {
	for _, run := range m.runs {
		if run.OrgID == orgID && run.Repo == repo && run.PRNumber == prNumber &&
			(run.Status == "queued" || run.Status == "running") {
			clone := *run
			return &clone, nil
		}
	}
	return nil, nil
}

func (m *mockReviewRunRepository) ClaimDueRuns(_ context.Context, limit int, lease time.Duration) ([]model.ReviewRun, error) {
	now := time.Now()
	claimed := make([]model.ReviewRun, 0)
	for _, run := range m.runs {
		if run.Status != "queued" && run.Status != "running" {
			continue
		}
		if run.NextPollAt != nil && run.NextPollAt.After(now) {
			continue
		}
		next := now.Add(lease)
		run.NextPollAt = &next
		claimed = append(claimed, *run)
		if len(claimed) >= limit {
			break
		}
	}
	return claimed, nil
}

func testReviewCfg() reviewbot.Config {
	return reviewbot.Config{
		Enabled:      true,
		BaseURL:      "http://jobshout-review-bot:8765",
		AllowedRepos: []string{"bwalia/jobshout"},
		PollInterval: time.Second,
		MaxRuntime:   time.Hour,
	}
}

func TestReviewServiceCreateRunDefaultsRealRunAndQueues(t *testing.T) {
	repo := newMockReviewRunRepository()
	svc := NewReviewService(repo, testReviewCfg(), zap.NewNop())
	orgID := uuid.New()

	run, err := svc.CreateRun(context.Background(), model.CreateReviewRunRequest{
		Repo: "bwalia/jobshout", PRNumber: 12,
	}, orgID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.DryRun {
		t.Fatal("dry_run should default false")
	}
	if run.Status != "queued" {
		t.Fatalf("status = %s", run.Status)
	}
}

func TestReviewServiceCreateRunRejectsUnknownRepo(t *testing.T) {
	svc := NewReviewService(newMockReviewRunRepository(), testReviewCfg(), zap.NewNop())
	_, err := svc.CreateRun(context.Background(), model.CreateReviewRunRequest{
		Repo: "evil/repo", PRNumber: 1,
	}, uuid.New(), nil)
	if !errors.Is(err, ErrReviewRepoNotAllowed) {
		t.Fatalf("got %v", err)
	}
}

func TestReviewServiceCreateRunDisabled(t *testing.T) {
	svc := NewReviewService(newMockReviewRunRepository(), reviewbot.Config{}, zap.NewNop())
	_, err := svc.CreateRun(context.Background(), model.CreateReviewRunRequest{
		Repo: "bwalia/jobshout", PRNumber: 1,
	}, uuid.New(), nil)
	if !errors.Is(err, ErrReviewNotConfigured) {
		t.Fatalf("got %v", err)
	}
}

func TestReviewServiceCreateRunReturnsActive(t *testing.T) {
	repo := newMockReviewRunRepository()
	svc := NewReviewService(repo, testReviewCfg(), zap.NewNop())
	orgID := uuid.New()
	first, err := svc.CreateRun(context.Background(), model.CreateReviewRunRequest{
		Repo: "bwalia/jobshout", PRNumber: 9,
	}, orgID, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateRun(context.Background(), model.CreateReviewRunRequest{
		Repo: "bwalia/jobshout", PRNumber: 9,
	}, orgID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the active run, got %s vs %s", first.ID, second.ID)
	}
}

func TestReviewServiceCreateRunBindsTaskIDOnReuse(t *testing.T) {
	repo := newMockReviewRunRepository()
	svc := NewReviewService(repo, testReviewCfg(), zap.NewNop())
	orgID := uuid.New()
	first, err := svc.CreateRun(context.Background(), model.CreateReviewRunRequest{
		Repo: "bwalia/jobshout", PRNumber: 11,
	}, orgID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.TaskID != nil {
		t.Fatal("first run should have no task yet")
	}
	taskID := uuid.New()
	second, err := svc.CreateRun(context.Background(), model.CreateReviewRunRequest{
		Repo: "bwalia/jobshout", PRNumber: 11, TaskID: &taskID,
	}, orgID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the active run, got %s vs %s", first.ID, second.ID)
	}
	if second.TaskID == nil || *second.TaskID != taskID {
		t.Fatalf("reused run must bind the board task, got %v", second.TaskID)
	}
	stored := repo.runs[first.ID]
	if stored.TaskID == nil || *stored.TaskID != taskID {
		t.Fatalf("persisted task_id = %v", stored.TaskID)
	}
}

func TestReviewServiceGetRunHidesOtherOrgs(t *testing.T) {
	repo := newMockReviewRunRepository()
	svc := NewReviewService(repo, testReviewCfg(), zap.NewNop())
	run, err := svc.CreateRun(context.Background(), model.CreateReviewRunRequest{
		Repo: "bwalia/jobshout", PRNumber: 3,
	}, uuid.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.GetRun(context.Background(), run.ID, uuid.New())
	if !errors.Is(err, ErrReviewRunNotFound) {
		t.Fatalf("got %v", err)
	}
}

type fakeReviewClient struct {
	startFn     func(ctx context.Context, req reviewbot.StartRequest) (*reviewbot.Handle, error)
	statusFn    func(ctx context.Context, jobID string) (*reviewbot.Snapshot, error)
	enabled     bool
	startCalls  int
	statusCalls int
	lastRunRef  string
}

func (f *fakeReviewClient) Start(ctx context.Context, req reviewbot.StartRequest) (*reviewbot.Handle, error) {
	f.startCalls++
	f.lastRunRef = req.RunRef
	return f.startFn(ctx, req)
}

func (f *fakeReviewClient) Status(ctx context.Context, jobID string) (*reviewbot.Snapshot, error) {
	f.statusCalls++
	return f.statusFn(ctx, jobID)
}

func (f *fakeReviewClient) Enabled() bool { return f.enabled }

func seedQueuedReview(repo *mockReviewRunRepository) *model.ReviewRun {
	now := time.Now()
	run := &model.ReviewRun{
		ID:         uuid.New(),
		OrgID:      uuid.New(),
		Repo:       "bwalia/jobshout",
		PRNumber:   4,
		DryRun:     true,
		Status:     "queued",
		NextPollAt: &now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	repo.runs[run.ID] = run
	return run
}

func newTestReviewReconciler(repo *mockReviewRunRepository, client ReviewBotClient) *ReviewReconciler {
	rc := NewReviewReconciler(repo, client, reviewbot.Config{PollInterval: time.Second, MaxRuntime: time.Hour}, zap.NewNop())
	rc.interval = 0
	rc.backoff = 0
	return rc
}

func TestReviewReconcilerHappyPath(t *testing.T) {
	ctx := context.Background()
	repo := newMockReviewRunRepository()
	run := seedQueuedReview(repo)
	result, _ := json.Marshal(map[string]any{
		"decision": "FIX", "verdict": "needs work", "summary": "a bug", "head_sha": "abc",
	})
	client := &fakeReviewClient{
		enabled: true,
		startFn: func(ctx context.Context, req reviewbot.StartRequest) (*reviewbot.Handle, error) {
			if req.RunRef != run.ID.String() {
				t.Errorf("run_ref = %s", req.RunRef)
			}
			return &reviewbot.Handle{JobID: "job1", State: "queued"}, nil
		},
	}
	client.statusFn = func(ctx context.Context, id string) (*reviewbot.Snapshot, error) {
		if client.statusCalls < 2 {
			return &reviewbot.Snapshot{JobID: id, State: "running", StageLog: []string{"working"}}, nil
		}
		return &reviewbot.Snapshot{JobID: id, State: "done", Result: result, StageLog: []string{"working", "done"}}, nil
	}

	rc := newTestReviewReconciler(repo, client)
	if err := rc.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	got := repo.runs[run.ID]
	if got.Status != "running" || got.RemoteJobID == nil || *got.RemoteJobID != "job1" {
		t.Fatalf("after start: %+v", got)
	}
	if client.lastRunRef != run.ID.String() {
		t.Fatalf("run_ref = %s", client.lastRunRef)
	}

	if err := rc.Tick(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if repo.runs[run.ID].Status != "running" {
		t.Fatalf("want still running, got %s", repo.runs[run.ID].Status)
	}

	if err := rc.Tick(ctx); err != nil {
		t.Fatalf("tick 3: %v", err)
	}
	final := repo.runs[run.ID]
	if final.Status != "completed" {
		t.Fatalf("want completed, got %s", final.Status)
	}
	if final.Decision == nil || *final.Decision != "FIX" {
		t.Fatalf("decision = %v", final.Decision)
	}
	if final.NextPollAt != nil {
		t.Fatal("completed run must not poll again")
	}
	if client.startCalls != 1 {
		t.Fatalf("Start called %d times", client.startCalls)
	}
}

func TestReviewReconcilerSidecarRestartFailsRow(t *testing.T) {
	ctx := context.Background()
	repo := newMockReviewRunRepository()
	run := seedQueuedReview(repo)
	jobID := "lost"
	run.RemoteJobID = &jobID
	run.Status = "running"

	client := &fakeReviewClient{
		enabled: true,
		startFn: func(ctx context.Context, req reviewbot.StartRequest) (*reviewbot.Handle, error) {
			t.Fatal("must not start again")
			return nil, nil
		},
		statusFn: func(ctx context.Context, jobID string) (*reviewbot.Snapshot, error) {
			return nil, reviewbot.ErrJobNotFound
		},
	}
	rc := newTestReviewReconciler(repo, client)
	if err := rc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got := repo.runs[run.ID]
	if got.Status != "failed" {
		t.Fatalf("status = %s", got.Status)
	}
	if got.ErrorMessage == nil || !strings.Contains(*got.ErrorMessage, "restarted") {
		t.Fatalf("error = %v", got.ErrorMessage)
	}
}
