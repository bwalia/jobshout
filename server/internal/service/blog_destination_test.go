package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/model"
)

func TestOpsapiDestination(t *testing.T) {
	ok := model.Integration{
		Name:        "Workstation Website",
		Provider:    model.ProviderOpsAPI,
		BaseURL:     "https://opsapi.workstation.co.uk/",
		Credentials: map[string]any{"api_key": strings.Join([]string{"opsk", "test"}, "_")},
		Config:      map[string]any{"namespace_slug": "workstation"},
	}
	dest, err := opsapiDestination(ok)
	if err != nil {
		t.Fatalf("valid integration: %v", err)
	}
	if dest.Namespace() != "workstation" {
		t.Errorf("namespace = %q", dest.Namespace())
	}

	broken := map[string]func(*model.Integration){
		"provider":  func(i *model.Integration) { i.Provider = model.ProviderJira },
		"inactive":  func(i *model.Integration) { i.Status = "disabled" },
		"base_url":  func(i *model.Integration) { i.BaseURL = "ftp://x" },
		"api_key":   func(i *model.Integration) { i.Credentials = map[string]any{} },
		"namespace": func(i *model.Integration) { i.Config = map[string]any{} },
		// Left sealed by a server without its SECRETS_ENCRYPTION_KEY.
		"sealed key": func(i *model.Integration) { i.Credentials = map[string]any{"api_key": "enc:v1:AAAA"} },
	}
	for name, mut := range broken {
		in := ok
		mut(&in)
		if _, err := opsapiDestination(in); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// Runs pick a website by integration name, case-insensitively; the namespace
// comes from that integration, so two websites never share a key.
func TestFindDestination(t *testing.T) {
	key := func(s string) map[string]any {
		return map[string]any{"api_key": strings.Join([]string{"opsk", s}, "_")}
	}
	list := []model.Integration{
		{Name: "Workstation Website", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Credentials: key("ws"), Config: map[string]any{"namespace_slug": "workstation"}},
		{Name: "JobShout", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Credentials: key("js"), Config: map[string]any{"namespace_slug": "jobshout"}},
		{Name: "Half done", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Config: map[string]any{"namespace_slug": "x"}},
	}
	for name, ns := range map[string]string{
		"jobshout": "jobshout", " Workstation Website ": "workstation",
		// The New Task form names a namespace rather than a website.
		"workstation": "workstation", "JOBSHOUT": "jobshout",
	} {
		d, err := findDestination(list, name, "")
		if err != nil || d.Namespace() != ns {
			t.Errorf("%q: namespace %v, err %v; want %s", name, d, err, ns)
		}
	}
	for _, bad := range []string{"Nowhere", "Half done", "x"} {
		if _, err := findDestination(list, bad, ""); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
	if _, err := findDestination(list, "brand-new", ""); !errors.Is(err, errNoDestination) {
		t.Errorf("unsaved namespace: err %v, want errNoDestination", err)
	}
}

// One namespace saved on two OpsAPIs: the ring's wins, and without a ring
// URL to break the tie the run must name the website.
func TestFindDestination_NamespaceOnTwoOpsAPIs(t *testing.T) {
	key := map[string]any{"api_key": strings.Join([]string{"opsk", "k"}, "_")}
	list := []model.Integration{
		{Name: "Elsewhere", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.example.com",
			Credentials: key, Config: map[string]any{"namespace_slug": "jobshout"}},
		{Name: "Here", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Credentials: key, Config: map[string]any{"namespace_slug": "jobshout"}},
	}
	d, err := findDestination(list, "jobshout", "https://OPSAPI.workstation.co.uk/")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.(interface{ BaseURL() string }).BaseURL(); got != "https://opsapi.workstation.co.uk" {
		t.Errorf("picked %s, want the ring's OpsAPI", got)
	}
	if _, err := findDestination(list, "jobshout", ""); err == nil || errors.Is(err, errNoDestination) {
		t.Errorf("ambiguous namespace: err %v, want a pick-by-name error", err)
	}
}

type listIntegrations []model.Integration

func (l listIntegrations) ListByOrg(context.Context, uuid.UUID) ([]model.Integration, error) {
	return l, nil
}

// The add-website form asks for a key only for a namespace the org has not
// saved; the answer never carries the stored key.
func TestCheckWebsite(t *testing.T) {
	key := strings.Join([]string{"opsk", "stored", "abcdef"}, "_")
	svc := &blogService{integrations: listIntegrations{
		{ID: uuid.New(), Name: "JobShout", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk/",
			Credentials: map[string]any{"api_key": key}, Config: map[string]any{"namespace_slug": "jobshout"}},
		{ID: uuid.New(), Name: "No key", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Config: map[string]any{"namespace_slug": "halfdone"}},
	}}
	ctx, org := context.Background(), uuid.New()
	base := "https://OPSAPI.workstation.co.uk"

	got, err := svc.CheckWebsite(ctx, org, " JobShout ", base)
	if err != nil || !got.Exists || !got.Ready || got.Name != "JobShout" || got.IntegrationID == nil {
		t.Fatalf("saved namespace = %+v, %v; want exists and ready", got, err)
	}
	if got.Namespace != "jobshout" {
		t.Errorf("namespace not normalised: %q", got.Namespace)
	}

	if got, _ := svc.CheckWebsite(ctx, org, "halfdone", base); !got.Exists || got.Ready {
		t.Errorf("saved without a key = %+v; want exists, not ready", got)
	}
	if got, _ := svc.CheckWebsite(ctx, org, "workstation", base); got.Exists {
		t.Errorf("new namespace = %+v; want not exists", got)
	}
	// Same namespace on another OpsAPI is another website.
	if got, _ := svc.CheckWebsite(ctx, org, "jobshout", "https://opsapi.example.com"); got.Exists {
		t.Errorf("other OpsAPI = %+v; want not exists", got)
	}

	if got, _ := svc.CheckWebsite(ctx, org, "jobshout", base); got.Ring {
		t.Errorf("saved namespace = %+v; want the saved website, not the ring's", got)
	}

	for _, bad := range [][2]string{{"Job Shout", base}, {"", base}, {"-x", base}, {"jobshout", ""}, {"jobshout", "ftp://x"}} {
		if _, err := svc.CheckWebsite(ctx, org, bad[0], bad[1]); !errors.Is(err, ErrInvalidIntegration) {
			t.Errorf("%q on %q: err %v, want ErrInvalidIntegration", bad[0], bad[1], err)
		}
	}
}

// The ring's own CMS namespace ("acme" for liveCMS) is connected already:
// the form reuses the ring's key rather than asking for one, but only on the
// ring's OpsAPI.
func TestCheckWebsite_RingNamespace(t *testing.T) {
	svc := &blogService{
		runner:        publishingRunner(&liveCMS{}),
		integrations:  listIntegrations{},
		opsapiBaseURL: "https://opsapi.workstation.co.uk",
	}
	ctx, org := context.Background(), uuid.New()
	got, err := svc.CheckWebsite(ctx, org, "acme", "https://opsapi.workstation.co.uk/")
	if err != nil || !got.Exists || !got.Ready || !got.Ring || got.IntegrationID != nil {
		t.Fatalf("ring namespace = %+v, %v; want exists, ready, ring", got, err)
	}
	if got, _ := svc.CheckWebsite(ctx, org, "acme", "https://opsapi.example.com"); got.Exists {
		t.Errorf("ring namespace on another OpsAPI = %+v; want not exists", got)
	}
	// A ring without a CMS has no namespace of its own.
	svc.runner = blog.NewRunner(blog.Config{}, nil, nil, nil, zap.NewNop())
	if got, _ := svc.CheckWebsite(ctx, org, "acme", "https://opsapi.workstation.co.uk"); got.Exists {
		t.Errorf("no ring CMS = %+v; want not exists", got)
	}
}

// fakeOpsAPI records the posts it is sent: the namespace header and key.
type fakeOpsAPI struct {
	mu    sync.Mutex
	posts []string // "namespace|key"
}

func (f *fakeOpsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.posts = append(f.posts, r.Header.Get("X-Namespace-Slug")+"|"+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	f.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]string{"uuid": uuid.NewString(), "status": "draft"}})
}

// A run launched for a namespace files its articles there: a saved
// namespace with its stored key, the ring's own namespace with the ring's
// CMS, and an unconnected one not at all — it is refused before writing.
func TestPublish_ToSelectedNamespace(t *testing.T) {
	ops := &fakeOpsAPI{}
	srv := httptest.NewServer(ops)
	defer srv.Close()
	stored := strings.Join([]string{"opsk", "stored", "jobshout"}, "_")
	integrations := listIntegrations{
		{ID: uuid.New(), Name: "JobShout site", Provider: model.ProviderOpsAPI, BaseURL: srv.URL,
			Credentials: map[string]any{"api_key": stored}, Config: map[string]any{"namespace_slug": "jobshout"}},
	}
	org := uuid.New()
	publish := func(t *testing.T, namespace string) (*liveCMS, error) {
		t.Helper()
		run := aRun(org, model.BlogRunStatusCompleted, "one")
		run.Options.Destination = namespace
		store := &liveStore{runStore{run: run, storedArticles: []model.BlogArticle{
			{ID: uuid.New(), RunID: run.ID, Topic: "one", Slug: "one", Title: "One", Markdown: "# One\n\nA.", HTML: "<h1>One</h1><p>A.</p>"},
		}}}
		ring := &liveCMS{}
		svc := &blogService{
			runner: publishingRunner(ring),
			repo:   store, logger: zap.NewNop(),
			integrations: integrations, opsapiBaseURL: srv.URL,
		}
		_, err := svc.Publish(context.Background(), org, run.ID)
		return ring, err
	}

	t.Run("existing namespace reuses its stored key", func(t *testing.T) {
		ring, err := publish(t, "jobshout")
		if err != nil {
			t.Fatal(err)
		}
		if len(ops.posts) != 1 || ops.posts[0] != "jobshout|"+stored {
			t.Fatalf("OpsAPI got %v, want one post to jobshout with the stored key", ops.posts)
		}
		if len(ring.created) != 0 {
			t.Errorf("ring CMS got %d posts, want none", len(ring.created))
		}
	})

	t.Run("ring namespace uses the ring's CMS", func(t *testing.T) {
		ops.posts = nil
		ring, err := publish(t, "acme")
		if err != nil {
			t.Fatal(err)
		}
		if len(ring.created) != 1 || len(ops.posts) != 0 {
			t.Fatalf("ring CMS got %d, saved site got %d; want 1 and 0", len(ring.created), len(ops.posts))
		}
	})

	t.Run("new namespace without a key is refused", func(t *testing.T) {
		ops.posts = nil
		ring, err := publish(t, "brand-new")
		if err == nil {
			t.Fatal("want an error for an unconnected namespace")
		}
		if len(ring.created) != 0 || len(ops.posts) != 0 {
			t.Fatalf("posts were sent for an unconnected namespace: ring %d, opsapi %d", len(ring.created), len(ops.posts))
		}
		svc := &blogService{integrations: integrations, opsapiBaseURL: srv.URL}
		if _, err := svc.resolveDestination(context.Background(), org, "brand-new"); !errors.Is(err, errNoDestination) {
			t.Errorf("launch check: err %v, want errNoDestination", err)
		}
	})
}
