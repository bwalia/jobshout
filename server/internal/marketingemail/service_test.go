package marketingemail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

// fakePages is an in-memory CMS.
type fakePages struct {
	mu        sync.Mutex
	pages     map[string]*opsapi.Page // by uuid
	html      map[string]string
	creates   int
	updates   int
	updateErr error
	lastReq   opsapi.CreatePageRequest
	lastPatch any
}

func newFakePages() *fakePages {
	return &fakePages{pages: map[string]*opsapi.Page{}, html: map[string]string{}}
}

func (f *fakePages) CreatePage(_ context.Context, req opsapi.CreatePageRequest) (*opsapi.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	f.lastReq = req
	p := &opsapi.Page{UUID: fmt.Sprintf("page-%d", len(f.pages)+1), Title: req.Title, Slug: req.Slug, Status: req.Status, Template: req.Template}
	f.pages[p.UUID] = p
	f.html[p.UUID] = req.ContentHTML
	return p, nil
}

func (f *fakePages) UpdatePage(_ context.Context, id string, fields any) (*opsapi.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	f.lastPatch = fields
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	p, ok := f.pages[id]
	if !ok {
		return nil, errors.New("not found")
	}
	switch v := fields.(type) {
	case opsapi.CreatePageRequest:
		f.lastReq = v
		p.Title, p.Status, p.Template = v.Title, v.Status, v.Template
		f.html[id] = v.ContentHTML
	case map[string]string:
		if s, ok := v["status"]; ok {
			p.Status = s
		}
	}
	return p, nil
}

func (f *fakePages) FindPageBySlug(_ context.Context, slug string) (*opsapi.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.pages {
		if p.Slug == slug {
			return p, nil
		}
	}
	return nil, nil
}

type fakeResolver struct{ dest *Destination }

func (r fakeResolver) Resolve(context.Context, uuid.UUID, string) (*Destination, error) {
	if r.dest == nil {
		return nil, ErrDestination
	}
	return r.dest, nil
}

// memRuns is an in-memory run store.
type memRuns struct {
	mu   sync.Mutex
	runs map[uuid.UUID]*model.MarketingEmailRun
}

func (m *memRuns) Create(_ context.Context, r *model.MarketingEmailRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *r
	m.runs[r.ID] = &c
	return nil
}
func (m *memRuns) Save(_ context.Context, r *model.MarketingEmailRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *r
	m.runs[r.ID] = &c
	return nil
}
func (m *memRuns) Get(_ context.Context, _, id uuid.UUID) (*model.MarketingEmailRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.runs[id]; ok {
		c := *r
		return &c, nil
	}
	return nil, errors.New("not found")
}
func (m *memRuns) List(context.Context, uuid.UUID, int) ([]model.MarketingEmailRun, error) {
	return nil, nil
}
func (m *memRuns) FailStale(context.Context, time.Time, string) (int, error) { return 0, nil }

type fakeIllustrator struct {
	url     string
	enabled bool
	called  int
}

func (f *fakeIllustrator) Enabled() bool { return f.enabled }
func (f *fakeIllustrator) Generate(context.Context, IllustrationRequest) (*Illustration, error) {
	f.called++
	return &Illustration{URL: f.url}, nil
}

func newTestService(t *testing.T, pages *fakePages, replies ...string) (*Service, *memRuns) {
	t.Helper()
	runs := &memRuns{runs: map[uuid.UUID]*model.MarketingEmailRun{}}
	dest := &Destination{Pages: pages, BaseURL: "https://int-opsapi.workstation.co.uk", Namespace: "workstation"}
	s := NewService(&Writer{LLM: &fakeCompleter{replies: replies}}, fakeResolver{dest: dest}, nil)
	s.BindRuns(runs)
	s.WithLogoChecker(nil) // no network: the header is the text wordmark
	s.sync = true
	return s, runs
}

func startReq(brief string) StartRequest {
	return StartRequest{OrgID: uuid.New(), UserID: uuid.New(), Brief: model.MarketingEmailBrief{Brief: brief}}
}

func TestStart_PublishesLiveEmailPage(t *testing.T) {
	pages := newFakePages()
	s, runs := newTestService(t, pages, goodJSON)
	ill := &fakeIllustrator{enabled: true, url: "https://img.example/hero.png"}
	s.WithIllustrator(ill)

	id, err := s.Start(context.Background(), startReq("Workstation AI shop is live"))
	if err != nil {
		t.Fatal(err)
	}
	run := runs.runs[id]
	if run.Status != model.MarketingEmailStatusPublished {
		t.Fatalf("status = %s (%s)", run.Status, run.Error)
	}
	if run.Slug != "ai-shop-live" || run.PageUUID == "" || run.PageStatus != "published" {
		t.Errorf("run = slug %q page %q status %q", run.Slug, run.PageUUID, run.PageStatus)
	}
	if run.BrowserURL != "https://int.workstation.co.uk/email/ai-shop-live" {
		t.Errorf("browser url = %q", run.BrowserURL)
	}
	req := pages.lastReq
	if req.Template != "email" || req.Status != "published" || req.ShowInNav == nil || *req.ShowInNav {
		t.Errorf("page request = %+v", req)
	}
	if req.SEOTitle != run.Subject || req.SEODescription != run.Preheader || req.Excerpt != run.Preheader {
		t.Errorf("seo/excerpt not set from subject/preheader: %+v", req)
	}
	if req.FeaturedImageURL != "https://img.example/hero.png" || !strings.Contains(req.ContentHTML, "https://img.example/hero.png") {
		t.Errorf("hero image not used")
	}
	if strings.Contains(req.ContentHTML, "{{") {
		t.Error("published html has a merge tag")
	}
}

func TestStart_DraftModeFilesDraft(t *testing.T) {
	pages := newFakePages()
	s, runs := newTestService(t, pages, goodJSON)
	req := startReq("brief text here")
	req.Brief.Publish = "draft"
	id, err := s.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if run := runs.runs[id]; run.Status != model.MarketingEmailStatusDraft || pages.lastReq.Status != "draft" {
		t.Errorf("status = %s, page status = %s", run.Status, pages.lastReq.Status)
	}
}

func TestStart_NewRunsGetUniqueSlugs_ReGenerateUpserts(t *testing.T) {
	pages := newFakePages()
	s, runs := newTestService(t, pages, goodJSON, goodJSON, goodJSON)

	id1, _ := s.Start(context.Background(), startReq("first"))
	id2, _ := s.Start(context.Background(), startReq("second"))
	if runs.runs[id1].Slug != "ai-shop-live" || runs.runs[id2].Slug != "ai-shop-live-2" {
		t.Fatalf("slugs = %q, %q", runs.runs[id1].Slug, runs.runs[id2].Slug)
	}
	if pages.creates != 2 {
		t.Fatalf("creates = %d", pages.creates)
	}

	// Regenerate the first: same page updated, no third page.
	run, err := s.ReGenerate(context.Background(), runs.runs[id1].OrgID, id1)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != model.MarketingEmailStatusPublished {
		t.Fatalf("status = %s (%s)", run.Status, run.Error)
	}
	if pages.creates != 2 || pages.updates != 1 || len(pages.pages) != 2 {
		t.Errorf("creates=%d updates=%d pages=%d; want an update of the same page", pages.creates, pages.updates, len(pages.pages))
	}
	if runs.runs[id1].Slug != "ai-shop-live" {
		t.Errorf("slug changed to %q", runs.runs[id1].Slug)
	}
}

func TestReGenerate_ForbiddenUpdateIsClearAndCreatesNothing(t *testing.T) {
	pages := newFakePages()
	s, runs := newTestService(t, pages, goodJSON, goodJSON)
	id, _ := s.Start(context.Background(), startReq("first"))
	pages.updateErr = fmt.Errorf("%w: update page forbidden (403) — the API key needs the cms:update scope", opsapi.ErrUpdateForbidden)

	run, err := s.ReGenerate(context.Background(), runs.runs[id].OrgID, id)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != model.MarketingEmailStatusFailed || !strings.Contains(run.Error, "cms:update") {
		t.Errorf("status = %s error = %q", run.Status, run.Error)
	}
	if pages.creates != 1 || len(pages.pages) != 1 {
		t.Errorf("a duplicate page was created: creates=%d pages=%d", pages.creates, len(pages.pages))
	}
}

func TestUnpublish_ArchivesPage(t *testing.T) {
	pages := newFakePages()
	s, runs := newTestService(t, pages, goodJSON)
	id, _ := s.Start(context.Background(), startReq("first"))
	orgID := runs.runs[id].OrgID

	run, err := s.Unpublish(context.Background(), orgID, id)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != model.MarketingEmailStatusArchived || run.PageStatus != "archived" {
		t.Errorf("run = %s / %s", run.Status, run.PageStatus)
	}
	if got := pages.pages[run.PageUUID].Status; got != "archived" {
		t.Errorf("cms status = %q", got)
	}
}

func TestUnpublish_NothingPublished(t *testing.T) {
	s, runs := newTestService(t, newFakePages())
	id := uuid.New()
	runs.runs[id] = &model.MarketingEmailRun{ID: id, Status: model.MarketingEmailStatusFailed}
	if _, err := s.Unpublish(context.Background(), uuid.New(), id); !errors.Is(err, ErrNotPublished) {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_PageOwnedByNonEmailTemplateIsNotOverwritten(t *testing.T) {
	pages := newFakePages()
	pages.pages["x"] = &opsapi.Page{UUID: "x", Slug: "ai-shop-live", Template: "default", Status: "published"}
	s, runs := newTestService(t, pages, goodJSON)
	// ReGenerate path keeps the slug, so seed a run that already owns it.
	id := uuid.New()
	orgID := uuid.New()
	runs.runs[id] = &model.MarketingEmailRun{ID: id, OrgID: orgID, Slug: "ai-shop-live", Status: model.MarketingEmailStatusPublished,
		Brief: model.MarketingEmailBrief{Brief: "b"}, PublishMode: "live"}
	run, err := s.ReGenerate(context.Background(), orgID, id)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != model.MarketingEmailStatusFailed || pages.updates != 0 {
		t.Errorf("status = %s updates = %d", run.Status, pages.updates)
	}
}

func TestHero_OnlyAbsoluteHTTPS(t *testing.T) {
	for _, tt := range []struct {
		name    string
		ill     *fakeIllustrator
		wantImg bool
	}{
		{"https", &fakeIllustrator{enabled: true, url: "https://img.example/a.png"}, true},
		{"relative", &fakeIllustrator{enabled: true, url: "/api/v1/images/a.png"}, false},
		{"http", &fakeIllustrator{enabled: true, url: "http://img.example/a.png"}, false},
		{"disabled", &fakeIllustrator{enabled: false, url: "https://img.example/a.png"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pages := newFakePages()
			s, runs := newTestService(t, pages, goodJSON)
			s.WithIllustrator(tt.ill)
			id, _ := s.Start(context.Background(), startReq("brief"))
			got := runs.runs[id].HeroImageURL != ""
			if got != tt.wantImg {
				t.Errorf("hero = %q, want image %v", runs.runs[id].HeroImageURL, tt.wantImg)
			}
			if !tt.wantImg && strings.Count(pages.lastReq.ContentHTML, "<img ") != 0 {
				t.Error("html has a hero image it should not")
			}
		})
	}
}

func TestStart_RejectsBadLaunch(t *testing.T) {
	s, _ := newTestService(t, newFakePages())
	for name, mut := range map[string]func(*StartRequest){
		"empty brief": func(r *StartRequest) { r.Brief.Brief = "  " },
		"http cta":    func(r *StartRequest) { r.Brief.PrimaryCTAURL = "http://x.example" },
		"bad goal":    func(r *StartRequest) { r.Brief.Goal = "spam" },
		"bad publish": func(r *StartRequest) { r.Brief.Publish = "later" },
	} {
		req := startReq("a brief")
		mut(&req)
		if _, err := s.Start(context.Background(), req); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	// Unresolvable destination is an immediate error too.
	s.destinations = fakeResolver{}
	if _, err := s.Start(context.Background(), startReq("a brief")); !errors.Is(err, ErrDestination) {
		t.Errorf("err = %v", err)
	}
}

func TestStart_DraftFailureFailsRun(t *testing.T) {
	s, runs := newTestService(t, newFakePages(), "not json at all", "still not json")
	id, err := s.Start(context.Background(), startReq("brief"))
	if err != nil {
		t.Fatal(err)
	}
	if run := runs.runs[id]; run.Status != model.MarketingEmailStatusFailed || run.Error == "" {
		t.Errorf("run = %s / %q", run.Status, run.Error)
	}
}
