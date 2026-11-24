package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

// resendCMS is a fake opsapi client that can create and update posts.
type resendCMS struct {
	mu      sync.Mutex
	created []opsapi.CreatePostRequest
	updated map[string]opsapi.UpdatePostRequest
	// existing is the set of post uuids the CMS still has.
	existing  map[string]bool
	updateErr error
	// gate, when set, holds every CMS call until it is closed.
	gate chan struct{}
}

func newResendCMS(existing ...string) *resendCMS {
	c := &resendCMS{updated: map[string]opsapi.UpdatePostRequest{}, existing: map[string]bool{}}
	for _, id := range existing {
		c.existing[id] = true
	}
	return c
}

func (c *resendCMS) Namespace() string { return "acme" }

func (c *resendCMS) wait() {
	if c.gate != nil {
		<-c.gate
	}
}

func (c *resendCMS) CreatePost(_ context.Context, req opsapi.CreatePostRequest) (*opsapi.Post, error) {
	c.wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.created = append(c.created, req)
	id := fmt.Sprintf("new-%d", len(c.created))
	c.existing[id] = true
	return &opsapi.Post{UUID: id, Slug: req.Slug, Status: req.Status, Title: req.Title}, nil
}

func (c *resendCMS) UpdatePost(_ context.Context, id string, req opsapi.UpdatePostRequest) (*opsapi.Post, error) {
	c.wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updateErr != nil {
		return nil, c.updateErr
	}
	if !c.existing[id] {
		// What the real client returns for opsapi's 404.
		return nil, fmt.Errorf("%w: %s in namespace %q", opsapi.ErrPostNotFound, id, "acme")
	}
	c.updated[id] = req
	return &opsapi.Post{UUID: id, Status: opsapi.StatusPublished, Title: req.Title}, nil
}

// resendStore holds one run and its articles, with the resend claim kept the
// way the database keeps it: one row, one claim at a time.
type resendStore struct {
	liveStore
	claimed  bool
	claims   int
	finished []model.ArticleResend
}

func (s *resendStore) GetArticle(_ context.Context, id uuid.UUID) (*model.BlogArticle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.storedArticles {
		if a.ID == id {
			cp := a
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *resendStore) ClaimArticleResend(context.Context, uuid.UUID, time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed {
		return false, nil
	}
	s.claimed = true
	s.claims++
	return true, nil
}

func (s *resendStore) FinishArticleResend(_ context.Context, id uuid.UUID, rec model.ArticleResend, post *model.BlogArticlePost) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimed = false
	s.finished = append(s.finished, rec)
	for i := range s.storedArticles {
		if s.storedArticles[i].ID != id {
			continue
		}
		r := rec
		s.storedArticles[i].LastResend = &r
		if post != nil {
			pid, st := post.PostUUID, post.Status
			s.storedArticles[i].PostUUID, s.storedArticles[i].PostStatus = &pid, &st
		}
	}
	return nil
}

// resendFixture is an org, a completed run with one article (postUUID "" =
// never filed), a CMS and the service wired to them.
func resendFixture(t *testing.T, postUUID string, cms *resendCMS) (*blogService, *resendStore, uuid.UUID, uuid.UUID) {
	t.Helper()
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusCompleted, "one")
	covers := newMemCovers()
	cover, err := covers.PutFile(context.Background(), org, []byte("png"), "png", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	a := model.BlogArticle{ID: uuid.New(), RunID: run.ID, OrgID: org, Topic: "one", Slug: "one", Title: "One",
		Markdown: "# One\n\nA.", HTML: "<h1>One</h1><p>A.</p>", CoverImageURL: cover}
	if postUUID != "" {
		st := opsapi.StatusDraft
		a.PostUUID, a.PostStatus = &postUUID, &st
	}
	store := &resendStore{liveStore: liveStore{runStore{run: run, storedArticles: []model.BlogArticle{a}}}}
	runner := blog.NewRunner(blog.Config{PublicBaseURL: "https://jobshout.test"}, nil, cms, nil, zap.NewNop()).
		WithCoverStore(covers)
	return &blogService{runner: runner, repo: store, logger: zap.NewNop()}, store, org, a.ID
}

func TestResendArticle_UpdatesTheExistingPost(t *testing.T) {
	cms := newResendCMS("post-1")
	svc, store, org, id := resendFixture(t, "post-1", cms)

	got, err := svc.ResendArticle(context.Background(), org, id)
	if err != nil {
		t.Fatalf("resend: %v", err)
	}
	if len(cms.created) != 0 {
		t.Fatalf("created %d posts, want 0: an article with a post must update it", len(cms.created))
	}
	req, ok := cms.updated["post-1"]
	if !ok || req.ContentHTML == "" || req.Title != "One" || req.FeaturedImageURL == "" {
		t.Fatalf("post-1 not updated with the article: %+v", req)
	}
	if got.LastResend == nil || got.LastResend.Result != model.ResendResultUpdated || got.LastResend.PostUUID != "post-1" {
		t.Fatalf("last resend = %+v", got.LastResend)
	}
	// The status the CMS reports now (an editor took it live) is recorded.
	if *got.PostUUID != "post-1" || *got.PostStatus != opsapi.StatusPublished {
		t.Fatalf("post = %s/%s", *got.PostUUID, *got.PostStatus)
	}
	if store.claimed {
		t.Fatal("claim not released")
	}
}

func TestResendArticle_RecreatesADeletedPost(t *testing.T) {
	cms := newResendCMS() // the CMS no longer has post-1
	svc, _, org, id := resendFixture(t, "post-1", cms)

	got, err := svc.ResendArticle(context.Background(), org, id)
	if err != nil {
		t.Fatalf("resend: %v", err)
	}
	if len(cms.created) != 1 || cms.created[0].Status != opsapi.StatusDraft {
		t.Fatalf("created = %+v, want one draft", cms.created)
	}
	if got.LastResend.Result != model.ResendResultCreated || *got.PostUUID != "new-1" {
		t.Fatalf("result=%s post=%s, want created new-1", got.LastResend.Result, *got.PostUUID)
	}
}

func TestResendArticle_FilesANeverPublishedArticle(t *testing.T) {
	cms := newResendCMS()
	svc, _, org, id := resendFixture(t, "", cms)

	got, err := svc.ResendArticle(context.Background(), org, id)
	if err != nil {
		t.Fatalf("resend: %v", err)
	}
	if len(cms.created) != 1 || len(cms.updated) != 0 {
		t.Fatalf("created=%d updated=%d", len(cms.created), len(cms.updated))
	}
	if got.LastResend.Result != model.ResendResultCreated || *got.PostUUID != "new-1" {
		t.Fatalf("last resend = %+v", got.LastResend)
	}
}

// Resend sends exactly what Publish files: same endpoint, same payload.
func TestResendArticle_SendsWhatPublishSends(t *testing.T) {
	cms := newResendCMS()
	svc, store, org, id := resendFixture(t, "", cms)

	if _, err := svc.Publish(context.Background(), org, store.run.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// Forget the post, as if it had never been filed, and resend.
	store.storedArticles[0].PostUUID = nil
	if _, err := svc.ResendArticle(context.Background(), org, id); err != nil {
		t.Fatalf("resend: %v", err)
	}
	if len(cms.created) != 2 {
		t.Fatalf("created %d posts", len(cms.created))
	}
	if !reflect.DeepEqual(cms.created[0], cms.created[1]) {
		t.Fatalf("payloads differ:\npublish %+v\nresend  %+v", cms.created[0], cms.created[1])
	}
}

func TestResendArticle_RecordsAFailure(t *testing.T) {
	cms := newResendCMS("post-1")
	cms.updateErr = errors.New("opsapi: cms update forbidden")
	svc, store, org, id := resendFixture(t, "post-1", cms)

	if _, err := svc.ResendArticle(context.Background(), org, id); err == nil {
		t.Fatal("want the CMS error")
	}
	if len(cms.created) != 0 {
		t.Fatal("a refused update must not file a duplicate draft")
	}
	if len(store.finished) != 1 {
		t.Fatalf("finished %d times", len(store.finished))
	}
	rec := store.finished[0]
	if rec.Result != model.ResendResultFailed || rec.Error == "" || rec.PostUUID != "post-1" || rec.At.IsZero() {
		t.Fatalf("recorded %+v", rec)
	}
	if store.claimed {
		t.Fatal("claim not released after a failure")
	}
}

func TestResendArticle_RefusedWhileAnotherIsInFlight(t *testing.T) {
	cms := newResendCMS()
	svc, store, org, id := resendFixture(t, "", cms)
	store.claimed = true

	if _, err := svc.ResendArticle(context.Background(), org, id); !errors.Is(err, ErrResendInProgress) {
		t.Fatalf("err = %v, want ErrResendInProgress", err)
	}
	if len(cms.created) != 0 {
		t.Fatal("CMS called while another resend held the claim")
	}
}

// Two resends at once (double click, two tabs): one files, the other is refused.
func TestResendArticle_ConcurrentResendsFileOnce(t *testing.T) {
	cms := newResendCMS()
	cms.gate = make(chan struct{})
	svc, _, org, id := resendFixture(t, "", cms)

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := svc.ResendArticle(context.Background(), org, id)
			errs <- err
		}()
	}
	// The loser returns without reaching the CMS; then let the winner through.
	first := <-errs
	close(cms.gate)
	second := <-errs

	inFlight := 0
	for _, err := range []error{first, second} {
		if errors.Is(err, ErrResendInProgress) {
			inFlight++
		} else if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if inFlight != 1 || len(cms.created) != 1 {
		t.Fatalf("refused=%d created=%d, want 1 and 1", inFlight, len(cms.created))
	}
}

func TestResendArticle_RefusesAnotherOrgsArticle(t *testing.T) {
	cms := newResendCMS()
	svc, store, _, id := resendFixture(t, "", cms)

	if _, err := svc.ResendArticle(context.Background(), uuid.New(), id); !errors.Is(err, ErrArticleOtherOrg) {
		t.Fatalf("err = %v", err)
	}
	if store.claims != 0 || len(cms.created) != 0 {
		t.Fatal("another org's article was resent")
	}
}
