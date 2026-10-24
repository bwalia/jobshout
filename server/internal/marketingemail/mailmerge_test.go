package marketingemail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

func TestRenderMailMerge_KeepsAllowedMergeFields(t *testing.T) {
	d := goodDraft()
	// Model text can neither smuggle a sentinel nor a merge tag through.
	d.Sections[0].BodyMarkdown = "See https://jsmergetag.invalid/unsubscribe_url and {{secret}} jsmergetagfirstname"
	out, err := RenderMailMerge(d, "", logoChrome)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Hi {{contact.first_name}},",
		`href="{{unsubscribe_url}}"`,
		`href="{{preferences_url}}"`,
		`href="{{view_online_url}}"`,
		"Trouble viewing?", "View this email in your browser",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("mail-merge html missing %q", want)
		}
	}
	allowed := map[string]bool{}
	for _, k := range AllowedMergeFields {
		allowed[k] = true
	}
	for _, k := range MergeKeys(out) {
		if !allowed[k] {
			t.Errorf("merge field {{%s}} is not allowed", k)
		}
	}
	for _, bad := range []string{"{{secret}}", "jsmergetag", "%7b%7b", "%7B%7B"} {
		if strings.Contains(out, bad) {
			t.Errorf("mail-merge html contains %q", bad)
		}
	}
}

func TestRender_BrowserVersionUsesDefaults(t *testing.T) {
	out, err := Render(goodDraft(), "", Chrome{PreferencesURL: "https://esp.example/prefs", UnsubscribeURL: "https://esp.example/unsub"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Hi there,", `href="https://esp.example/prefs"`, `href="https://esp.example/unsub"`} {
		if !strings.Contains(out, want) {
			t.Errorf("browser html missing %q", want)
		}
	}
	if strings.Contains(out, "Trouble viewing?") || strings.Contains(out, "{{") {
		t.Error("browser version must have no view-online line and no merge tag")
	}
}

func TestRender_WordmarkWithoutLogo(t *testing.T) {
	out, err := Render(goodDraft(), "", Chrome{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<img ") {
		t.Error("no logo URL must mean no image at all")
	}
	if !strings.Contains(out, ">WORKSTATION</a>") || !strings.Contains(out, "letter-spacing:4px;color:#e5165f") {
		t.Error("wordmark missing")
	}
}

func TestRenderMailMergeText(t *testing.T) {
	txt := RenderMailMergeText(goodDraft(), Chrome{})
	for _, want := range []string{"{{view_online_url}}", "Hi {{contact.first_name}},", "{{unsubscribe_url}}", "{{preferences_url}}"} {
		if !strings.Contains(txt, want) {
			t.Errorf("text missing %q", want)
		}
	}
	if strings.Contains(txt, "<") || strings.Contains(txt, "**") {
		t.Errorf("text has markup:\n%s", txt)
	}
}

func TestLogoChecker(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/ok.png":
			w.WriteHeader(http.StatusOK)
		case "/nohead.png":
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	now := time.Now()
	c := &LogoChecker{HTTP: srv.Client(), now: func() time.Time { return now }}
	ctx := context.Background()
	if !c.Available(ctx, srv.URL+"/ok.png") || !c.Available(ctx, srv.URL+"/nohead.png") {
		t.Fatal("a 200 logo must be available")
	}
	if c.Available(ctx, srv.URL+"/missing.png") || c.Available(ctx, "http://insecure.example/logo.png") || c.Available(ctx, "") {
		t.Fatal("a missing or non-https logo must not be available")
	}
	before := hits.Load()
	c.Available(ctx, srv.URL+"/ok.png")
	c.Available(ctx, srv.URL+"/missing.png")
	if hits.Load() != before {
		t.Error("results must be cached")
	}
	now = now.Add(logoMissingTTL + time.Second)
	c.Available(ctx, srv.URL+"/missing.png")
	if hits.Load() == before {
		t.Error("a missing logo must be checked again after its TTL")
	}
	var nilChecker *LogoChecker
	if nilChecker.Available(ctx, srv.URL+"/ok.png") {
		t.Error("a nil checker means no logo")
	}
}

// fakeTemplates is an in-memory OpsAPI Marketing template store.
type fakeTemplates struct {
	mu      sync.Mutex
	byUUID  map[string]*opsapi.MarketingTemplate
	html    map[string]string
	creates int
	saves   int
	err     error
}

func newFakeTemplates() *fakeTemplates {
	return &fakeTemplates{byUUID: map[string]*opsapi.MarketingTemplate{}, html: map[string]string{}}
}

func (f *fakeTemplates) CreateMarketingTemplate(_ context.Context, req opsapi.MarketingTemplateRequest) (*opsapi.MarketingTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.creates++
	t := &opsapi.MarketingTemplate{UUID: fmt.Sprintf("tpl-%d", len(f.byUUID)+1), Name: req.Name, Channel: req.Channel, Status: req.Status}
	f.byUUID[t.UUID] = t
	return t, nil
}

func (f *fakeTemplates) SaveMarketingTemplate(_ context.Context, id string, req opsapi.MarketingTemplateRequest) (*opsapi.MarketingTemplate, *opsapi.TemplateAnalysis, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, nil, f.err
	}
	t, ok := f.byUUID[id]
	if !ok {
		return nil, nil, fmt.Errorf("opsapi: save template %s: not found", id)
	}
	f.saves++
	if req.Status != "" {
		t.Status = req.Status
	}
	f.html[id] = req.BodyHTML
	return t, &opsapi.TemplateAnalysis{OK: true, MergeFields: MergeKeys(req.BodyHTML)}, nil
}

func (f *fakeTemplates) FindMarketingTemplateByName(_ context.Context, name string) (*opsapi.MarketingTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	for _, t := range f.byUUID {
		if strings.EqualFold(t.Name, name) {
			return t, nil
		}
	}
	return nil, nil
}

func newTemplateService(t *testing.T, replies ...string) (*Service, *memRuns, *fakeTemplates) {
	t.Helper()
	s, runs := newTestService(t, newFakePages(), replies...)
	tpls := newFakeTemplates()
	s.destinations.(fakeResolver).dest.Templates = tpls
	s.WithTemplateSave(true)
	return s, runs, tpls
}

func TestRun_SavesMailMergeTemplate_UpsertByName(t *testing.T) {
	s, runs, tpls := newTemplateService(t, goodJSON, goodJSON)
	id, err := s.Start(context.Background(), startReq("Workstation AI shop is live"))
	if err != nil {
		t.Fatal(err)
	}
	run := runs.runs[id]
	if run.Status != model.MarketingEmailStatusPublished || run.TemplateOutcome != model.MarketingTemplateSaved {
		t.Fatalf("status = %s template = %s (%s)", run.Status, run.TemplateOutcome, run.TemplateMessage)
	}
	if run.TemplateUUID != "tpl-1" || run.TemplateName != "Email: ai-shop-live" || tpls.byUUID["tpl-1"].Status != "draft" {
		t.Errorf("template = %q %q %+v", run.TemplateUUID, run.TemplateName, tpls.byUUID["tpl-1"])
	}
	html := tpls.html["tpl-1"]
	if !strings.Contains(html, "{{contact.first_name}}") || !strings.Contains(html, "{{view_online_url}}") || run.TemplateHTML != html {
		t.Error("template html lacks the merge fields")
	}
	if strings.Contains(run.ContentHTML, "{{") {
		t.Error("the browser version must keep no merge tag")
	}

	// Re-generating updates the same template rather than creating another.
	if _, err := s.ReGenerate(context.Background(), run.OrgID, id); err != nil {
		t.Fatal(err)
	}
	if tpls.creates != 1 || tpls.saves != 2 || runs.runs[id].TemplateUUID != "tpl-1" {
		t.Errorf("creates=%d saves=%d uuid=%q", tpls.creates, tpls.saves, runs.runs[id].TemplateUUID)
	}
}

func TestRun_TemplateScopeMissingDoesNotFailPublish(t *testing.T) {
	s, runs, tpls := newTemplateService(t, goodJSON)
	tpls.err = fmt.Errorf("opsapi: find template forbidden (403): %w", opsapi.ErrMarketingScope)
	id, _ := s.Start(context.Background(), startReq("brief text"))
	run := runs.runs[id]
	if run.Status != model.MarketingEmailStatusPublished || run.PageUUID == "" {
		t.Fatalf("the CMS publish must still succeed: %s (%s)", run.Status, run.Error)
	}
	if run.TemplateOutcome != model.MarketingTemplateNotSaved ||
		run.TemplateMessage != "Template not saved: missing opsapi_marketing_templates scope" {
		t.Errorf("outcome = %s message = %q", run.TemplateOutcome, run.TemplateMessage)
	}
}

func TestRun_TemplateSaveDisabled(t *testing.T) {
	s, runs, tpls := newTemplateService(t, goodJSON)
	s.WithTemplateSave(false)
	id, _ := s.Start(context.Background(), startReq("brief text"))
	if run := runs.runs[id]; run.TemplateOutcome != model.MarketingTemplateDisabled || tpls.creates != 0 {
		t.Errorf("outcome = %s creates = %d", run.TemplateOutcome, tpls.creates)
	}
}

func TestSaveTemplate_RetriesFromStoredDraft(t *testing.T) {
	s, runs, tpls := newTemplateService(t, goodJSON)
	tpls.err = fmt.Errorf("x: %w", opsapi.ErrMarketingScope)
	id, _ := s.Start(context.Background(), startReq("brief text"))
	tpls.err = nil // the key gained the scope
	run, err := s.SaveTemplate(context.Background(), runs.runs[id].OrgID, id)
	if err != nil {
		t.Fatal(err)
	}
	if run.TemplateOutcome != model.MarketingTemplateSaved || run.TemplateUUID == "" {
		t.Errorf("outcome = %s (%s)", run.TemplateOutcome, run.TemplateMessage)
	}
}

func TestSaveTemplatesFromEnv(t *testing.T) {
	t.Setenv(EnvSaveTemplate, "")
	if !SaveTemplatesFromEnv() {
		t.Error("default must be on")
	}
	t.Setenv(EnvSaveTemplate, "off")
	if SaveTemplatesFromEnv() {
		t.Error("off must turn it off")
	}
}

func TestDraftPrompt_TradeFactsOnlyForTrade(t *testing.T) {
	if p := draftPrompt(model.MarketingEmailBrief{Brief: "Join Workstation Trade membership"}); !strings.Contains(p, "£75/month") ||
		!strings.Contains(p, "+44 7515 356 146") {
		t.Error("a Trade brief must carry the Trade facts")
	}
	if p := draftPrompt(model.MarketingEmailBrief{Brief: "Mac Studio M5 is in stock"}); strings.Contains(p, "£75/month") {
		t.Error("a non-Trade brief must not carry the Trade facts")
	}
}
