package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/integration/adapters/jobshoutcom"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

// liveCMS drafts and changes status, recording each status change.
type liveCMS struct {
	statusCalls []string
	statusErr   error
}

func (c *liveCMS) Namespace() string { return "acme" }
func (c *liveCMS) CreatePost(_ context.Context, req opsapi.CreatePostRequest) (*opsapi.Post, error) {
	return &opsapi.Post{UUID: "new-" + req.Slug, Status: req.Status}, nil
}
func (c *liveCMS) SetPostStatus(_ context.Context, id, status string) (*opsapi.Post, error) {
	if c.statusErr != nil {
		return nil, c.statusErr
	}
	c.statusCalls = append(c.statusCalls, id+"="+status)
	return &opsapi.Post{UUID: id, Status: status}, nil
}

type liveJobshout struct {
	submitted []jobshoutcom.SubmitInsightRequest
	err       error
}

func (j *liveJobshout) SubmitInsight(_ context.Context, req jobshoutcom.SubmitInsightRequest) (*jobshoutcom.Insight, error) {
	if j.err != nil {
		return nil, j.err
	}
	j.submitted = append(j.submitted, req)
	return &jobshoutcom.Insight{ID: "item-1", Slug: "the-article", Status: "published"}, nil
}

// liveStore records what PublishLive marks on the articles.
type liveStore struct {
	runStore
}

func (s *liveStore) MarkArticlesPosted(_ context.Context, posts []model.BlogArticlePost) error {
	for _, p := range posts {
		for i := range s.storedArticles {
			if s.storedArticles[i].ID == p.ArticleID {
				id, st := p.PostUUID, p.Status
				s.storedArticles[i].PostUUID, s.storedArticles[i].PostStatus = &id, &st
			}
		}
	}
	return nil
}

func (s *liveStore) MarkArticlesInInsights(_ context.Context, items []model.BlogArticleInsights) error {
	for _, it := range items {
		for i := range s.storedArticles {
			if s.storedArticles[i].ID == it.ArticleID {
				id := it.ItemID
				s.storedArticles[i].InsightsItemID = &id
			}
		}
	}
	return nil
}

func liveFixture(writer string) (*blogService, *liveStore, *liveCMS, *liveJobshout, uuid.UUID) {
	org := uuid.New()
	published := time.Now()
	run := aRun(org, model.BlogRunStatusCompleted, "Decision models")
	run.Options.Writer = writer
	run.PublishedAt = &published
	postID, draft := "post-1", opsapi.StatusDraft
	store := &liveStore{runStore{run: run, storedArticles: []model.BlogArticle{{
		ID: uuid.New(), RunID: run.ID, Topic: "Decision models", Slug: "decision-models",
		Title: "Decision models", Markdown: "# Decision models\n\nBody.", PostUUID: &postID, PostStatus: &draft,
	}}}}
	cms, js := &liveCMS{}, &liveJobshout{}
	runner := blog.NewRunner(blog.Config{}, nil, cms, nil, zap.NewNop()).WithLiveInsights(js)
	return &blogService{runner: runner, repo: store, logger: zap.NewNop()}, store, cms, js, org
}

func TestPublishLive_RefusesArticleWriterRuns(t *testing.T) {
	svc, _, cms, js, org := liveFixture("")
	if _, err := svc.PublishLive(context.Background(), org, svc.repo.(*liveStore).run.ID); err == nil {
		t.Fatal("an Article Writer run must not be published live")
	}
	if len(cms.statusCalls) != 0 || len(js.submitted) != 0 {
		t.Fatal("nothing should have been sent")
	}
}

func TestPublishLive_PublishesCMSPostThenJobshoutCom(t *testing.T) {
	svc, store, cms, js, org := liveFixture(model.BuiltinJobShoutComWriter)
	run, err := svc.PublishLive(context.Background(), org, store.run.ID)
	if err != nil {
		t.Fatalf("publish live: %v", err)
	}
	if len(cms.statusCalls) != 1 || cms.statusCalls[0] != "post-1=published" {
		t.Fatalf("want the draft set live, got %v", cms.statusCalls)
	}
	if len(js.submitted) != 1 || js.submitted[0].Title != "Decision models" {
		t.Fatalf("want one jobshout.com submission, got %+v", js.submitted)
	}
	if run.InsightsPublishedAt == nil {
		t.Fatal("run should be stamped as published on jobshout.com")
	}
	for _, st := range run.Steps {
		if st.Key == model.BlogStepLive && st.Agent != model.AgentNameJobShoutComWriter {
			t.Fatalf("live step attributed to %q", st.Agent)
		}
	}
}

func TestPublishLive_RetryFinishesOnlyWhatFailed(t *testing.T) {
	svc, store, cms, js, org := liveFixture(model.BuiltinJobShoutComWriter)
	js.err = errors.New("jobshout.com down")
	if _, err := svc.PublishLive(context.Background(), org, store.run.ID); err == nil {
		t.Fatal("want the jobshout.com failure reported")
	}
	if len(cms.statusCalls) != 1 {
		t.Fatalf("the CMS post should have gone live before jobshout.com failed, got %v", cms.statusCalls)
	}

	js.err = nil
	if _, err := svc.PublishLive(context.Background(), org, store.run.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(cms.statusCalls) != 1 {
		t.Fatalf("a live post must not be set live again, got %v", cms.statusCalls)
	}
	if len(js.submitted) != 1 {
		t.Fatalf("want exactly one jobshout.com submission, got %d", len(js.submitted))
	}

	// A third press has nothing left to do and sends nothing.
	if _, err := svc.PublishLive(context.Background(), org, store.run.ID); err != nil {
		t.Fatalf("third press: %v", err)
	}
	if len(cms.statusCalls) != 1 || len(js.submitted) != 1 {
		t.Fatal("an already-live run must not be sent again")
	}
}

func TestCanPublishLive_NeedsBothHalves(t *testing.T) {
	onlyCMS := &blogService{runner: blog.NewRunner(blog.Config{}, nil, &liveCMS{}, nil, zap.NewNop())}
	if onlyCMS.CanPublishLive() {
		t.Fatal("without a jobshout.com client, publishing live is not configured")
	}
	svc, _, _, _, _ := liveFixture(model.BuiltinJobShoutComWriter)
	if !svc.CanPublishLive() {
		t.Fatal("with both halves, publishing live is configured")
	}
}

func TestRunWriterName_AttributesSteps(t *testing.T) {
	run := &model.BlogRun{Options: model.BlogRunOptions{Writer: model.BuiltinJobShoutComWriter}}
	steps := attributeSteps(initialSteps(true), runWriterName(run))
	var sawWriter bool
	for _, st := range steps {
		if st.Agent == model.AgentNameArticleWriter {
			t.Fatalf("step %q still attributed to the Article Writer", st.Key)
		}
		if st.Agent == model.AgentNameJobShoutComWriter {
			sawWriter = true
		}
	}
	if !sawWriter {
		t.Fatal("want writing steps attributed to the Content Writer")
	}
	if runWriterName(&model.BlogRun{}) != model.AgentNameArticleWriter {
		t.Fatal("a run with no writer is the Article Writer's")
	}
}
