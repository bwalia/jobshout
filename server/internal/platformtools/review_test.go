package platformtools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type fakeReviews struct {
	enabled bool
	allowed []string
	runs    []*model.ReviewRun
	err     error
}

func (f *fakeReviews) Enabled() bool          { return f.enabled }
func (f *fakeReviews) AllowedRepos() []string { return append([]string(nil), f.allowed...) }

func (f *fakeReviews) CreateRun(_ context.Context, req model.CreateReviewRunRequest, orgID uuid.UUID, _ *uuid.UUID) (*model.ReviewRun, error) {
	if f.err != nil {
		return nil, f.err
	}
	if !f.enabled {
		return nil, service.ErrReviewNotConfigured
	}
	dry := true
	if req.DryRun != nil {
		dry = *req.DryRun
	}
	run := &model.ReviewRun{
		ID:       uuid.New(),
		OrgID:    orgID,
		Repo:     req.Repo,
		PRNumber: req.PRNumber,
		DryRun:   dry,
		Force:    req.Force,
		Status:   "queued",
	}
	f.runs = append(f.runs, run)
	return run, nil
}

func (f *fakeReviews) GetRun(_ context.Context, runID, orgID uuid.UUID) (*model.ReviewRun, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, r := range f.runs {
		if r.ID == runID && r.OrgID == orgID {
			return r, nil
		}
	}
	return nil, service.ErrReviewRunNotFound
}

func (f *fakeReviews) ListRuns(_ context.Context, orgID uuid.UUID, _ model.PaginationParams) (*model.PaginatedResponse[model.ReviewRun], error) {
	var data []model.ReviewRun
	for _, r := range f.runs {
		if r.OrgID == orgID {
			data = append(data, *r)
		}
	}
	return &model.PaginatedResponse[model.ReviewRun]{Data: data, Total: len(data)}, nil
}

func TestParseRepoAndPR(t *testing.T) {
	repo, pr := parseRepoAndPR("https://github.com/bwalia/jobshout/pull/84", 0)
	if repo != "bwalia/jobshout" || pr != 84 {
		t.Fatalf("got %s #%d", repo, pr)
	}
	repo, pr = parseRepoAndPR("bwalia/jobshout", 12)
	if repo != "bwalia/jobshout" || pr != 12 {
		t.Fatalf("got %s #%d", repo, pr)
	}
}

func TestReviewPullRequest_QueuesDryRun(t *testing.T) {
	reviews := &fakeReviews{enabled: true, allowed: []string{"bwalia/jobshout"}}
	reg := NewRegistryWithTools(Deps{Reviews: reviews})
	tool, ok := reg.Get("review_pull_request")
	if !ok {
		t.Fatal("review_pull_request not registered")
	}
	org, user := uuid.New(), uuid.New()
	ctx := WithIdentity(context.Background(), Identity{OrgID: org, UserID: user})
	res, err := tool.Run(ctx, map[string]any{"repo": "bwalia/jobshout", "pr_number": 84})
	if err != nil {
		t.Fatal(err)
	}
	if res.Entity == nil || res.Entity.Kind != model.EntityReviewRun {
		t.Fatalf("entity = %+v", res.Entity)
	}
	data, _ := res.Data.(map[string]any)
	if data["status"] != "queued" || data["dry_run"] != true || data["pr"] != 84 {
		t.Fatalf("data = %+v", data)
	}
	if len(reviews.runs) != 1 {
		t.Fatalf("created %d runs", len(reviews.runs))
	}
}

func TestReviewPullRequest_ParsesGitHubURL(t *testing.T) {
	reviews := &fakeReviews{enabled: true}
	reg := NewRegistryWithTools(Deps{Reviews: reviews})
	tool, _ := reg.Get("review_pull_request")
	ctx := WithIdentity(context.Background(), Identity{OrgID: uuid.New(), UserID: uuid.New()})
	res, err := tool.Run(ctx, map[string]any{"repo": "https://github.com/bwalia/jobshout/pull/84"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := res.Data.(map[string]any)
	if data["repo"] != "bwalia/jobshout" || data["pr"] != 84 {
		t.Fatalf("data = %+v", data)
	}
}

func TestReviewPullRequest_AsksForPRNumber(t *testing.T) {
	reg := NewRegistryWithTools(Deps{Reviews: &fakeReviews{enabled: true}})
	tool, _ := reg.Get("review_pull_request")
	ctx := WithIdentity(context.Background(), Identity{OrgID: uuid.New(), UserID: uuid.New()})
	res, err := tool.Run(ctx, map[string]any{"repo": "bwalia/jobshout"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Missing) == 0 || res.Missing[0] != "pr_number" {
		t.Fatalf("missing = %v", res.Missing)
	}
}

func TestReviewPullRequest_RefusesUnknownRepo(t *testing.T) {
	reviews := &fakeReviews{enabled: true, allowed: []string{"bwalia/jobshout"}, err: service.ErrReviewRepoNotAllowed}
	reg := NewRegistryWithTools(Deps{Reviews: reviews})
	tool, _ := reg.Get("review_pull_request")
	ctx := WithIdentity(context.Background(), Identity{OrgID: uuid.New(), UserID: uuid.New()})
	res, err := tool.Run(ctx, map[string]any{"repo": "other/repo", "pr_number": 1})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := res.Data.(map[string]any)
	if data["refused"] != true {
		t.Fatalf("data = %+v", data)
	}
}

func TestReviewRunGet_ReturnsVerdict(t *testing.T) {
	org := uuid.New()
	run := &model.ReviewRun{
		ID: uuid.New(), OrgID: org, Repo: "bwalia/jobshout", PRNumber: 84, Status: "completed", DryRun: true,
		Result: json.RawMessage(`{"decision":"comment","summary":"Looks fine.","blocking":[{"title":"nil deref"}]}`),
	}
	reviews := &fakeReviews{enabled: true, runs: []*model.ReviewRun{run}}
	reg := NewRegistryWithTools(Deps{Reviews: reviews})
	tool, _ := reg.Get("review_run_get")
	ctx := WithIdentity(context.Background(), Identity{OrgID: org, UserID: uuid.New()})
	res, err := tool.Run(ctx, map[string]any{"run_id": run.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := res.Data.(map[string]any)
	if data["decision"] != "comment" || data["blocking_count"] != 1 {
		t.Fatalf("data = %+v", data)
	}
}

func TestReviewErr_Disabled(t *testing.T) {
	reg := NewRegistryWithTools(Deps{Reviews: &fakeReviews{enabled: false}})
	tool, _ := reg.Get("review_pull_request")
	ctx := WithIdentity(context.Background(), Identity{OrgID: uuid.New(), UserID: uuid.New()})
	res, err := tool.Run(ctx, map[string]any{"repo": "bwalia/jobshout", "pr_number": 1})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := res.Data.(map[string]any)
	if data["available"] != false {
		t.Fatalf("data = %+v", data)
	}
}

func TestAlwaysLoadIncludesReviewPullRequest(t *testing.T) {
	if !inAlwaysLoad("review_pull_request") || !inAlwaysLoad("review_run_get") {
		t.Fatal("PR review tools must be always-load so chat does not route them to agent_execute")
	}
}

func TestReviewErr_NotFound(t *testing.T) {
	_, err := reviewErr(Deps{}, errors.New("boom"))
	if err == nil {
		t.Fatal("expected passthrough")
	}
}
