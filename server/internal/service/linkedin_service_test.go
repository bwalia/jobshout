package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/linkedin"
	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// fakeLinkedInRepo is an in-memory LinkedInRepository with the same claim
// rules as the SQL: one row per (article, variant), guarded state changes.
type fakeLinkedInRepo struct {
	mu       sync.Mutex
	articles map[uuid.UUID]model.LinkedInArticle
	posts    map[uuid.UUID]*model.LinkedInPost
	conns    map[uuid.UUID]model.LinkedInConnection
	tokens   map[uuid.UUID][]byte
	states   map[string][2]uuid.UUID
	finished chan uuid.UUID
}

func newFakeLinkedInRepo() *fakeLinkedInRepo {
	return &fakeLinkedInRepo{
		articles: map[uuid.UUID]model.LinkedInArticle{},
		posts:    map[uuid.UUID]*model.LinkedInPost{},
		conns:    map[uuid.UUID]model.LinkedInConnection{},
		tokens:   map[uuid.UUID][]byte{},
		states:   map[string][2]uuid.UUID{},
		finished: make(chan uuid.UUID, 16),
	}
}

func (f *fakeLinkedInRepo) hasPosts(articleID uuid.UUID) bool {
	for _, p := range f.posts {
		if p.ArticleID == articleID {
			return true
		}
	}
	return false
}

func (f *fakeLinkedInRepo) FindArticle(_ context.Context, orgID, id uuid.UUID) (*model.LinkedInArticle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.articles[id]
	if !ok || a.OrgID != orgID {
		return nil, repository.ErrLinkedInNotFound
	}
	return &a, nil
}

func (f *fakeLinkedInRepo) FindArticleByTitle(_ context.Context, orgID uuid.UUID, words string) (*model.LinkedInArticle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.articles {
		if a.OrgID == orgID && strings.Contains(strings.ToLower(a.Title), strings.ToLower(words)) {
			return &a, nil
		}
	}
	return nil, repository.ErrLinkedInNotFound
}

func (f *fakeLinkedInRepo) LatestUndrafted(_ context.Context, orgID uuid.UUID) (*model.LinkedInArticle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *model.LinkedInArticle
	for _, a := range f.articles {
		a := a
		if a.OrgID == orgID && !f.hasPosts(a.ID) && (best == nil || a.PublishedAt.After(*best.PublishedAt)) {
			best = &a
		}
	}
	if best == nil {
		return nil, repository.ErrLinkedInNotFound
	}
	return best, nil
}

func (f *fakeLinkedInRepo) PublishedSince(_ context.Context, orgID uuid.UUID, since time.Time, limit int) ([]model.LinkedInArticle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.LinkedInArticle
	for _, a := range f.articles {
		if a.OrgID == orgID && a.PublishedAt.After(since) && !f.hasPosts(a.ID) && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeLinkedInRepo) ClaimDrafts(_ context.Context, p model.LinkedInPost, variants []string, redraft bool) ([]model.LinkedInPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.LinkedInPost
	for _, v := range variants {
		var existing *model.LinkedInPost
		for _, q := range f.posts {
			if q.ArticleID == p.ArticleID && q.Variant == v {
				existing = q
			}
		}
		switch {
		case existing == nil:
			row := p
			row.ID, row.Variant, row.Status, row.CreatedAt = uuid.New(), v, model.LinkedInPostDrafting, time.Now()
			f.posts[row.ID] = &row
			out = append(out, row)
		case redraft && (existing.Status == model.LinkedInPostDraft || existing.Status == model.LinkedInPostFailed):
			existing.Status, existing.Commentary, existing.ErrorMessage, existing.Notes = model.LinkedInPostDrafting, "", "", p.Notes
			out = append(out, *existing)
		}
	}
	return out, nil
}

func (f *fakeLinkedInRepo) FinishDraft(_ context.Context, id uuid.UUID, commentary, errMsg string) error {
	f.mu.Lock()
	if p := f.posts[id]; p != nil && p.Status == model.LinkedInPostDrafting {
		p.Commentary, p.ErrorMessage, p.Status = commentary, errMsg, model.LinkedInPostDraft
		if errMsg != "" {
			p.Status = model.LinkedInPostFailed
		}
	}
	f.mu.Unlock()
	f.finished <- id
	return nil
}

func (f *fakeLinkedInRepo) FailStaleDrafting(context.Context, time.Time) (int, error) { return 0, nil }

func (f *fakeLinkedInRepo) ListPosts(_ context.Context, orgID uuid.UUID, _ int) ([]model.LinkedInPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.LinkedInPost
	for _, p := range f.posts {
		if p.OrgID == orgID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (f *fakeLinkedInRepo) GetPost(_ context.Context, orgID, id uuid.UUID) (*model.LinkedInPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.posts[id]
	if p == nil || p.OrgID != orgID {
		return nil, repository.ErrLinkedInNotFound
	}
	c := *p
	return &c, nil
}

func (f *fakeLinkedInRepo) UpdateCommentary(_ context.Context, orgID, id uuid.UUID, commentary string) (*model.LinkedInPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.posts[id]
	if p == nil || p.OrgID != orgID {
		return nil, repository.ErrLinkedInNotFound
	}
	if p.Status != model.LinkedInPostDraft && p.Status != model.LinkedInPostFailed {
		return nil, repository.ErrLinkedInConflict
	}
	p.Commentary, p.Status = commentary, model.LinkedInPostDraft
	c := *p
	return &c, nil
}

func (f *fakeLinkedInRepo) ClaimPosting(_ context.Context, orgID, id uuid.UUID) (*model.LinkedInPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.posts[id]
	if p == nil || p.OrgID != orgID {
		return nil, repository.ErrLinkedInNotFound
	}
	if p.Status != model.LinkedInPostDraft || p.Commentary == "" {
		return nil, repository.ErrLinkedInConflict
	}
	p.Status = model.LinkedInPostPosting
	c := *p
	return &c, nil
}

func (f *fakeLinkedInRepo) MarkPosted(_ context.Context, id uuid.UUID, urn, url string, by *uuid.UUID) (*model.LinkedInPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.posts[id]
	p.Status, p.PostURN, p.PostURL, p.PostedBy = model.LinkedInPostPosted, urn, url, by
	c := *p
	return &c, nil
}

func (f *fakeLinkedInRepo) ReleasePosting(_ context.Context, id uuid.UUID, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p := f.posts[id]; p != nil && p.Status == model.LinkedInPostPosting {
		p.Status, p.ErrorMessage = model.LinkedInPostDraft, errMsg
	}
	return nil
}

func (f *fakeLinkedInRepo) GetConnection(_ context.Context, orgID uuid.UUID) (*model.LinkedInConnection, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.conns[orgID]
	if !ok {
		return nil, nil, repository.ErrLinkedInNotFound
	}
	return &c, f.tokens[orgID], nil
}

func (f *fakeLinkedInRepo) UpsertConnection(_ context.Context, c model.LinkedInConnection, enc []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conns[c.OrgID], f.tokens[c.OrgID] = c, enc
	return nil
}

func (f *fakeLinkedInRepo) DeleteConnection(_ context.Context, orgID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.conns, orgID)
	return nil
}

func (f *fakeLinkedInRepo) ConnectedOrgs(context.Context) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []uuid.UUID
	for id, c := range f.conns {
		if c.ExpiresAt.After(time.Now()) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeLinkedInRepo) PutOAuthState(_ context.Context, state string, orgID, userID uuid.UUID, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[state] = [2]uuid.UUID{orgID, userID}
	return nil
}

func (f *fakeLinkedInRepo) ConsumeOAuthState(_ context.Context, state string) (uuid.UUID, uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.states[state]
	if !ok {
		return uuid.Nil, uuid.Nil, repository.ErrLinkedInNotFound
	}
	delete(f.states, state)
	return v[0], v[1], nil
}

type fakeLinkedInAPI struct {
	postErr error
	posted  []linkedin.Post
	token   string
}

func (f *fakeLinkedInAPI) Exchange(_ context.Context, _, _, _, code string) (linkedin.Token, error) {
	return linkedin.Token{AccessToken: "tok-" + code, Expiry: time.Now().Add(60 * 24 * time.Hour)}, nil
}

func (f *fakeLinkedInAPI) Me(context.Context, string) (linkedin.Member, error) {
	return linkedin.Member{URN: "urn:li:person:me", Name: "Me"}, nil
}

func (f *fakeLinkedInAPI) CreatePost(_ context.Context, token string, p linkedin.Post) (string, error) {
	f.token = token
	if f.postErr != nil {
		return "", f.postErr
	}
	f.posted = append(f.posted, p)
	return "urn:li:share:1", nil
}

type fakeLinkedInDrafter struct{ fail string }

func (f *fakeLinkedInDrafter) Draft(_ context.Context, req linkedin.DraftRequest) (string, error) {
	if req.Variant == f.fail {
		return "", errors.New("model down")
	}
	return req.Variant + " post about " + req.Article.Title, nil
}

type fakeBuiltinAgents struct{ id uuid.UUID }

func (f fakeBuiltinAgents) FindBuiltin(context.Context, uuid.UUID, string) (*model.Agent, error) {
	if f.id == uuid.Nil {
		return nil, errors.New("not found")
	}
	return &model.Agent{ID: f.id}, nil
}

func newTestLinkedIn(t *testing.T, drafter linkedInDrafter) (*linkedInService, *fakeLinkedInRepo, *fakeLinkedInAPI) {
	t.Helper()
	repo := newFakeLinkedInRepo()
	api := &fakeLinkedInAPI{}
	cfg := linkedin.Config{
		ClientID: "cid", ClientSecret: "sec", TokenKey: "a passphrase", RedirectURL: "https://cb",
		AutoDraft: true, Lookback: 72 * time.Hour, DraftTimeout: time.Minute,
		InsightsSiteURL: "https://jobshout.com/",
	}
	svc := NewLinkedInService(repo, fakeBuiltinAgents{id: uuid.New()}, api, drafter, cfg, nil).(*linkedInService)
	return svc, repo, api
}

func addArticle(repo *fakeLinkedInRepo, orgID uuid.UUID, title, slug string, age time.Duration) model.LinkedInArticle {
	at := time.Now().Add(-age)
	a := model.LinkedInArticle{ID: uuid.New(), OrgID: orgID, Title: title, Markdown: "body", InsightsSlug: slug, PublishedAt: &at}
	repo.articles[a.ID] = a
	return a
}

func waitDrafts(t *testing.T, repo *fakeLinkedInRepo, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-repo.finished:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d drafts finished", i, n)
		}
	}
}

func TestLinkedInLaunchDraftsLatestArticleBothVariants(t *testing.T) {
	svc, repo, _ := newTestLinkedIn(t, &fakeLinkedInDrafter{fail: model.LinkedInVariantBusiness})
	org := uuid.New()
	addArticle(repo, org, "Older", "older", 48*time.Hour)
	a := addArticle(repo, org, "Fixing flaky deploys", "fixing-flaky-deploys", time.Hour)

	res, err := svc.Launch(context.Background(), linkedin.LaunchRequest{OrgID: org, AgentID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArticleID != a.ID || len(res.Variants) != 2 {
		t.Fatalf("result = %+v", res)
	}
	waitDrafts(t, repo, 2)

	posts, _ := repo.ListPosts(context.Background(), org, 10)
	byVariant := map[string]model.LinkedInPost{}
	for _, p := range posts {
		byVariant[p.Variant] = p
	}
	tech := byVariant[model.LinkedInVariantTechnical]
	if tech.Status != model.LinkedInPostDraft || tech.Commentary != "technical post about Fixing flaky deploys" {
		t.Errorf("technical = %+v", tech)
	}
	if tech.LinkURL != "https://jobshout.com/insights/fixing-flaky-deploys" {
		t.Errorf("link = %q", tech.LinkURL)
	}
	if biz := byVariant[model.LinkedInVariantBusiness]; biz.Status != model.LinkedInPostFailed || biz.ErrorMessage == "" {
		t.Errorf("business = %+v", biz)
	}

	// A second launch for the same article redrafts only what is not posted
	// or in flight; after posting both it has nothing left to draft.
	for _, p := range posts {
		repo.posts[p.ID].Status = model.LinkedInPostPosted
	}
	if _, err := svc.Launch(context.Background(), linkedin.LaunchRequest{OrgID: org, Article: a.ID.String()}); !errors.Is(err, ErrLinkedInNothingToDraft) {
		t.Errorf("relaunch after posting: err = %v", err)
	}
}

func TestLinkedInLaunchResolvesTitleAndReportsNoArticle(t *testing.T) {
	svc, repo, _ := newTestLinkedIn(t, &fakeLinkedInDrafter{})
	org := uuid.New()
	a := addArticle(repo, org, "Model routing economics", "", time.Hour)
	res, err := svc.Launch(context.Background(), linkedin.LaunchRequest{OrgID: org, Article: "routing", Variants: []string{model.LinkedInVariantTechnical}})
	if err != nil || res.ArticleID != a.ID || len(res.Variants) != 1 {
		t.Fatalf("Launch = %+v, %v", res, err)
	}
	waitDrafts(t, repo, 1)
	if _, err := svc.Launch(context.Background(), linkedin.LaunchRequest{OrgID: uuid.New()}); !errors.Is(err, ErrLinkedInNoArticle) {
		t.Errorf("empty org: err = %v", err)
	}
}

func TestLinkedInConnectThenPublish(t *testing.T) {
	svc, repo, api := newTestLinkedIn(t, &fakeLinkedInDrafter{})
	org, user := uuid.New(), uuid.New()

	if _, err := svc.Publish(context.Background(), org, user, uuid.New()); !errors.Is(err, ErrLinkedInNotConnected) {
		t.Fatalf("publish before connect: err = %v", err)
	}

	authURL, err := svc.StartConnect(context.Background(), org, user)
	if err != nil {
		t.Fatal(err)
	}
	var state string
	for s := range repo.states {
		state = s
	}
	if !strings.Contains(authURL, "state="+state) {
		t.Fatalf("auth URL %q lacks state %q", authURL, state)
	}
	if err := svc.CompleteConnect(context.Background(), state, "code1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteConnect(context.Background(), state, "code1"); err == nil {
		t.Error("state reused")
	}
	if string(repo.tokens[org]) == "tok-code1" {
		t.Fatal("token stored in the clear")
	}
	st, _ := svc.Status(context.Background(), org)
	if !st.Connected || st.Expired || st.DaysLeft < 59 || st.Name != "Me" {
		t.Errorf("status = %+v", st)
	}

	a := addArticle(repo, org, "Fixing flaky deploys", "fixing", time.Hour)
	claimed, _ := repo.ClaimDrafts(context.Background(), model.LinkedInPost{OrgID: org, ArticleID: a.ID, ArticleTitle: a.Title, LinkURL: "https://x"}, []string{model.LinkedInVariantTechnical}, false)
	id := claimed[0].ID
	if _, err := svc.Publish(context.Background(), org, user, id); !errors.Is(err, repository.ErrLinkedInConflict) {
		t.Errorf("publish while drafting: err = %v", err)
	}
	_ = repo.FinishDraft(context.Background(), id, "The post.", "")
	<-repo.finished

	posted, err := svc.Publish(context.Background(), org, user, id)
	if err != nil {
		t.Fatal(err)
	}
	if posted.Status != model.LinkedInPostPosted || posted.PostURN != "urn:li:share:1" || posted.PostURL == "" || *posted.PostedBy != user {
		t.Errorf("posted = %+v", posted)
	}
	if api.token != "tok-code1" || api.posted[0].AuthorURN != "urn:li:person:me" || api.posted[0].LinkTitle != a.Title {
		t.Errorf("call = token %q post %+v", api.token, api.posted)
	}
	if _, err := svc.Publish(context.Background(), org, user, id); !errors.Is(err, repository.ErrLinkedInConflict) {
		t.Errorf("second publish: err = %v (must not post twice)", err)
	}
}

func TestLinkedInPublishFailureReturnsDraftWithReason(t *testing.T) {
	svc, repo, api := newTestLinkedIn(t, &fakeLinkedInDrafter{})
	org := uuid.New()
	key, _ := mail.KeyFromSecret("a passphrase")
	enc, _ := mail.Encrypt(key, []byte("tok"))
	repo.conns[org] = model.LinkedInConnection{OrgID: org, MemberURN: "urn:li:person:me", ExpiresAt: time.Now().Add(time.Hour)}
	repo.tokens[org] = enc
	a := addArticle(repo, org, "T", "", time.Hour)
	claimed, _ := repo.ClaimDrafts(context.Background(), model.LinkedInPost{OrgID: org, ArticleID: a.ID}, []string{model.LinkedInVariantBusiness}, false)
	id := claimed[0].ID
	_ = repo.FinishDraft(context.Background(), id, "text", "")
	<-repo.finished

	api.postErr = linkedin.ErrNoAnswer
	if _, err := svc.Publish(context.Background(), org, uuid.New(), id); !errors.Is(err, linkedin.ErrNoAnswer) {
		t.Fatalf("err = %v", err)
	}
	p := repo.posts[id]
	if p.Status != model.LinkedInPostDraft || !strings.Contains(p.ErrorMessage, "check your LinkedIn feed") {
		t.Errorf("after no answer: %+v", p)
	}

	repo.conns[org] = model.LinkedInConnection{OrgID: org, MemberURN: "urn:li:person:me", ExpiresAt: time.Now().Add(-time.Minute)}
	if _, err := svc.Publish(context.Background(), org, uuid.New(), id); !errors.Is(err, linkedin.ErrTokenExpired) {
		t.Errorf("expired: err = %v", err)
	}
	if st, _ := svc.Status(context.Background(), org); !st.Expired || st.DaysLeft != 0 {
		t.Errorf("status = %+v", st)
	}
}

func TestLinkedInUpdatePostRejectsOverlongText(t *testing.T) {
	svc, repo, _ := newTestLinkedIn(t, &fakeLinkedInDrafter{})
	org := uuid.New()
	a := addArticle(repo, org, "T", "", time.Hour)
	claimed, _ := repo.ClaimDrafts(context.Background(), model.LinkedInPost{OrgID: org, ArticleID: a.ID}, []string{model.LinkedInVariantBusiness}, false)
	id := claimed[0].ID
	_ = repo.FinishDraft(context.Background(), id, "text", "")
	<-repo.finished
	if _, err := svc.UpdatePost(context.Background(), org, id, strings.Repeat("(", 1600)); err == nil {
		t.Error("text over the limit once escaped was accepted")
	}
	p, err := svc.UpdatePost(context.Background(), org, id, "  edited  ")
	if err != nil || p.Commentary != "edited" {
		t.Errorf("update = %+v, %v", p, err)
	}
}

func TestLinkedInAutoDraftOnlyConnectedOrgsAndNewArticles(t *testing.T) {
	svc, repo, _ := newTestLinkedIn(t, &fakeLinkedInDrafter{})
	connected, other := uuid.New(), uuid.New()
	repo.conns[connected] = model.LinkedInConnection{OrgID: connected, ExpiresAt: time.Now().Add(time.Hour)}
	fresh := addArticle(repo, connected, "Fresh", "fresh", time.Hour)
	addArticle(repo, connected, "Old", "old", 30*24*time.Hour)
	addArticle(repo, other, "Not connected", "nc", time.Hour)

	svc.autoDraftOnce(context.Background())
	waitDrafts(t, repo, 2)
	posts, _ := repo.ListPosts(context.Background(), connected, 10)
	if len(posts) != 2 || posts[0].ArticleID != fresh.ID {
		t.Fatalf("connected org posts = %+v", posts)
	}
	if others, _ := repo.ListPosts(context.Background(), other, 10); len(others) != 0 {
		t.Errorf("drafted for an org that never connected: %+v", others)
	}

	// A second tick drafts nothing new.
	svc.autoDraftOnce(context.Background())
	if posts, _ := repo.ListPosts(context.Background(), connected, 10); len(posts) != 2 {
		t.Errorf("second tick drafted again: %d posts", len(posts))
	}
}

func TestLinkedInNilDrafterIsNotConfigured(t *testing.T) {
	var d *linkedin.Drafter
	svc := NewLinkedInService(newFakeLinkedInRepo(), fakeBuiltinAgents{}, &fakeLinkedInAPI{}, d, linkedin.Config{}, nil)
	if _, err := svc.Launch(context.Background(), linkedin.LaunchRequest{OrgID: uuid.New()}); err == nil || !strings.Contains(err.Error(), "no LLM") {
		t.Errorf("err = %v", err)
	}
}
