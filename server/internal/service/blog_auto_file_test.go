package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/model"
)

// autoFileFixture is a completed run with one unposted article, a CMS and
// Insights that record what they receive, and BLOG_AUTO_FILE_CMS as given.
func autoFileFixture(autoFileCMS bool, cms blog.CMSPublisher) (*blogService, *liveStore, *liveJobshout, *model.BlogRun) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusCompleted, "Edge inference")
	store := &liveStore{runStore{run: run, storedArticles: []model.BlogArticle{{
		ID: uuid.New(), RunID: run.ID, Topic: "Edge inference", Slug: "edge-inference",
		Title: "Edge inference", Markdown: "# Edge inference\n\nBody.",
		HTML: "<h1>Edge inference</h1><p>Body.</p>",
	}}}}
	js := &liveJobshout{}
	runner := blog.NewRunner(blog.Config{}, nil, cms, nil, zap.NewNop()).WithInsights(js)
	svc := &blogService{runner: runner, repo: store, logger: zap.NewNop(), autoFileCMS: autoFileCMS}
	return svc, store, js, run
}

// The ring setting files a run launched without auto_publish — the Articles
// page, Task Manager and chat all launch that way — in the CMS only.
func TestFileCompletedRun_RingSettingFilesCMSOnly(t *testing.T) {
	cms := &liveCMS{}
	svc, store, js, run := autoFileFixture(true, cms)

	svc.fileCompletedRun(run, model.GenerateBlogRequest{}, zap.NewNop())

	if len(cms.created) != 1 {
		t.Fatalf("CMS received %d posts, want 1", len(cms.created))
	}
	if cms.created[0].Status != "draft" {
		t.Errorf("filed as %q, want a draft", cms.created[0].Status)
	}
	if len(js.submitted) != 0 {
		t.Errorf("Insights received %d items; the ring setting must not file there", len(js.submitted))
	}
	if store.run.PublishedAt == nil {
		t.Error("run not marked published after filing")
	}
	if a := store.storedArticles[0]; a.PostUUID == nil || *a.PostUUID == "" {
		t.Error("article has no post UUID recorded, so a later Publish would post it again")
	}
}

func TestFileCompletedRun_OffFilesNothing(t *testing.T) {
	cms := &liveCMS{}
	svc, _, js, run := autoFileFixture(false, cms)

	svc.fileCompletedRun(run, model.GenerateBlogRequest{}, zap.NewNop())

	if len(cms.created) != 0 || len(js.submitted) != 0 {
		t.Fatalf("filed %d CMS posts and %d Insights items with everything off, want none",
			len(cms.created), len(js.submitted))
	}
}

// auto_publish on a run is unchanged: the CMS and Insights both, once each,
// whether or not the ring also files every run.
func TestFileCompletedRun_AutoPublishUnchanged(t *testing.T) {
	for _, ring := range []bool{false, true} {
		cms := &liveCMS{}
		svc, _, js, run := autoFileFixture(ring, cms)

		svc.fileCompletedRun(run, model.GenerateBlogRequest{AutoPublish: true}, zap.NewNop())

		if len(cms.created) != 1 {
			t.Errorf("ring=%v: CMS received %d posts, want 1", ring, len(cms.created))
		}
		if len(js.submitted) != 1 {
			t.Errorf("ring=%v: Insights received %d items, want 1", ring, len(js.submitted))
		}
	}
}

// The Content Writer still reaches jobshout.com only when published live.
func TestFileCompletedRun_OtherWriterSkipsInsights(t *testing.T) {
	cms := &liveCMS{}
	svc, _, js, run := autoFileFixture(true, cms)

	svc.fileCompletedRun(run, model.GenerateBlogRequest{AutoPublish: true, Writer: model.BuiltinJobShoutComWriter}, zap.NewNop())

	if len(cms.created) != 1 {
		t.Errorf("CMS received %d posts, want 1", len(cms.created))
	}
	if len(js.submitted) != 0 {
		t.Errorf("Insights received %d items for the Content Writer, want 0", len(js.submitted))
	}
}

// Pressing Publish after the run was filed automatically must not post the
// article a second time.
func TestFileCompletedRun_ManualPublishAfterwardsDoesNotDuplicate(t *testing.T) {
	cms := &liveCMS{}
	svc, _, _, run := autoFileFixture(true, cms)

	svc.fileCompletedRun(run, model.GenerateBlogRequest{}, zap.NewNop())
	if _, err := svc.Publish(context.Background(), run.OrgID, run.ID); err == nil {
		t.Error("manual Publish of an already-filed run should be refused")
	}
	if len(cms.created) != 1 {
		t.Fatalf("CMS received %d posts, want 1 (no duplicate)", len(cms.created))
	}
}

// A CMS outage leaves the run completed and unpublished; the Publish button
// then files it, once.
func TestFileCompletedRun_FailureLeavesRunForManualPublish(t *testing.T) {
	cms := &flakyCMS{okCount: 0}
	svc, store, _, run := autoFileFixture(true, cms)

	svc.fileCompletedRun(run, model.GenerateBlogRequest{}, zap.NewNop())

	if store.run.Status != model.BlogRunStatusCompleted {
		t.Errorf("run status %q after a CMS failure, want it still completed", store.run.Status)
	}
	if store.run.PublishedAt != nil {
		t.Fatal("run marked published although the CMS refused it")
	}

	cms.okCount = 10
	if _, err := svc.Publish(context.Background(), run.OrgID, run.ID); err != nil {
		t.Fatalf("manual Publish after the outage: %v", err)
	}
	if len(cms.created) != 1 {
		t.Fatalf("CMS received %d posts, want 1", len(cms.created))
	}
}

// With no CMS configured the setting is inert rather than an error.
func TestFileCompletedRun_NoCMSConfigured(t *testing.T) {
	svc, store, js, run := autoFileFixture(true, nil)

	svc.fileCompletedRun(run, model.GenerateBlogRequest{}, zap.NewNop())

	if store.run.PublishedAt != nil || len(js.submitted) != 0 {
		t.Fatal("something was filed with no CMS configured")
	}
}
