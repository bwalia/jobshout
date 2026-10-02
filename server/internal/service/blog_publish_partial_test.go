package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

// flakyCMS accepts the first okCount posts and refuses the rest until healed.
type flakyCMS struct {
	liveCMS
	okCount int
}

func (c *flakyCMS) CreatePost(ctx context.Context, req opsapi.CreatePostRequest) (*opsapi.Post, error) {
	if len(c.created) >= c.okCount {
		return nil, errors.New("cms: 502 bad gateway")
	}
	return c.liveCMS.CreatePost(ctx, req)
}

// A publish that fails part-way must record the drafts it did create, so the
// retry posts only what is left instead of duplicating them in the CMS.
func TestPublish_RetryAfterPartialFailureDoesNotDuplicate(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusCompleted, "one", "two")
	store := &liveStore{runStore{run: run, storedArticles: []model.BlogArticle{
		{ID: uuid.New(), RunID: run.ID, Topic: "one", Slug: "one", Title: "One", Markdown: "# One\n\nA.", HTML: "<h1>One</h1><p>A.</p>"},
		{ID: uuid.New(), RunID: run.ID, Topic: "two", Slug: "two", Title: "Two", Markdown: "# Two\n\nB.", HTML: "<h1>Two</h1><p>B.</p>"},
	}}}
	cms := &flakyCMS{okCount: 1}
	svc := &blogService{runner: blog.NewRunner(blog.Config{}, nil, cms, nil, zap.NewNop()), repo: store, logger: zap.NewNop()}

	if _, err := svc.Publish(context.Background(), org, run.ID); err == nil {
		t.Fatal("first publish should fail on the second article")
	}
	cms.okCount = 10
	if _, err := svc.Publish(context.Background(), org, run.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(cms.created) != 2 {
		t.Fatalf("CMS received %d posts across both attempts, want 2 (no duplicate)", len(cms.created))
	}
}
