package marketingmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// fakeOpsAPI is a minimal Marketing Cloud: it records every call and answers
// from handlers keyed "METHOD /path".
type fakeOpsAPI struct {
	t        *testing.T
	mu       sync.Mutex
	calls    []string
	headers  []http.Header
	dryRun   bool
	handlers map[string]http.HandlerFunc
	srv      *httptest.Server
}

func newFake(t *testing.T) *fakeOpsAPI {
	f := &fakeOpsAPI{t: t, dryRun: true, handlers: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.headers = append(f.headers, r.Header.Clone())
		f.mu.Unlock()
		if h, ok := f.handlers[r.Method+" "+r.URL.Path]; ok {
			h(w, r)
			return
		}
		if r.URL.Path == marketingPrefix+"/sending" {
			ok(w, map[string]any{"via": "workspace", "dry_run": f.dryRun, "from_email": "news@example.com"})
			return
		}
		ok(w, map[string]any{"uuid": "x1", "path": r.URL.Path})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func ok(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

func (f *fakeOpsAPI) called(call string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == call {
			n++
		}
	}
	return n
}

// Built at runtime so secret scanners do not flag a fixture.
var testKey = strings.Join([]string{"opsk", "test", "secret", "value"}, "_")

func (f *fakeOpsAPI) service(testers ...string) *Service {
	return &Service{
		Ring: RingConfig{BaseURL: f.srv.URL, Key: testKey, Namespace: "workstation", TestRecipients: testers, Orgs: []uuid.UUID{org}},
	}
}

var org = uuid.New()

func TestRunRefusesActionOfAnotherKind(t *testing.T) {
	f := newFake(t)
	_, err := f.service().Run(context.Background(), org, Read, Call{Action: "schedule_campaign", ID: "c1"})
	if err == nil || !strings.Contains(err.Error(), "marketing_send") {
		t.Fatalf("want kind refusal, got %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("OpsAPI was called: %v", f.calls)
	}
}

func TestLiveSendGate(t *testing.T) {
	sched := "POST " + marketingPrefix + "/campaigns/c1/schedule"
	cases := []struct {
		name     string
		dryRun   bool
		allow    bool
		approved bool
		wantMode string
	}{
		{"test mode sends", true, false, false, ModeDryRun},
		{"live blocked on server", false, false, true, ""},
		{"live needs approval", false, true, false, ""},
		{"live approved", false, true, true, ModeLive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			f.dryRun = tc.dryRun
			s := f.service()
			s.AllowLiveSend = tc.allow
			res, err := s.Run(context.Background(), org, Send, Call{Action: "schedule_campaign", ID: "c1", LiveSendApproved: tc.approved})
			if tc.wantMode == "" {
				if !errors.Is(err, ErrLiveSendBlocked) {
					t.Fatalf("want ErrLiveSendBlocked, got %v", err)
				}
				if f.called(sched) != 0 {
					t.Fatal("schedule reached OpsAPI although blocked")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.SendMode != tc.wantMode || f.called(sched) != 1 {
				t.Fatalf("mode %q, schedule calls %d", res.SendMode, f.called(sched))
			}
		})
	}
}

func TestGateTreatsUnreadableTestModeAsLive(t *testing.T) {
	f := newFake(t)
	f.handlers["GET "+marketingPrefix+"/sending"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"API key is not scoped for this endpoint"}`))
	}
	_, err := f.service().Run(context.Background(), org, Send, Call{Action: "resume_campaign", ID: "c1"})
	if !errors.Is(err, ErrLiveSendBlocked) || f.called("POST "+marketingPrefix+"/campaigns/c1/resume") != 0 {
		t.Fatalf("want blocked without calling resume, got %v", err)
	}
}

func TestTestSendOnlyToConfirmedRecipients(t *testing.T) {
	path := "POST " + marketingPrefix + "/campaigns/c1/test-send"
	t.Run("no testers configured", func(t *testing.T) {
		f := newFake(t)
		_, err := f.service().Run(context.Background(), org, Send, Call{Action: "test_send", ID: "c1", Body: map[string]any{"email": "a@example.com"}})
		if !errors.Is(err, ErrTestRecipient) || f.called(path) != 0 {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("not a tester", func(t *testing.T) {
		f := newFake(t)
		_, err := f.service("qa@tenthmatrix.co.uk").Run(context.Background(), org, Send, Call{Action: "test_send", ID: "c1", Body: map[string]any{"email": "customer@example.com"}})
		if !errors.Is(err, ErrTestRecipient) || f.called(path) != 0 {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("contact who is not a tester", func(t *testing.T) {
		f := newFake(t)
		f.handlers["GET /api/v2/crm/contacts/ct1"] = func(w http.ResponseWriter, _ *http.Request) {
			ok(w, map[string]any{"uuid": "ct1", "email": "customer@example.com"})
		}
		_, err := f.service("qa@tenthmatrix.co.uk").Run(context.Background(), org, Send, Call{Action: "test_send", ID: "c1", Body: map[string]any{"contact_uuid": "ct1"}})
		if !errors.Is(err, ErrTestRecipient) || f.called(path) != 0 {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("tester, case-insensitive", func(t *testing.T) {
		f := newFake(t)
		res, err := f.service("QA@tenthmatrix.co.uk").Run(context.Background(), org, Send, Call{Action: "test_send", ID: "c1", Body: map[string]any{"email": " qa@TenthMatrix.co.uk "}})
		if err != nil || f.called(path) != 1 || res.SendMode != ModeDryRun {
			t.Fatalf("err %v, calls %d", err, f.called(path))
		}
	})
}

func TestCreateContactValidatesAndDedupes(t *testing.T) {
	post := "POST /api/v2/crm/contacts"
	f := newFake(t)
	f.handlers["GET /api/v2/crm/contacts"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") == "jo@example.com" {
			ok(w, []map[string]any{{"uuid": "ct9", "email": "Jo@Example.com"}})
			return
		}
		ok(w, []map[string]any{})
	}
	s := f.service()

	if _, err := s.Run(context.Background(), org, Write, Call{Action: "create_contact", Body: map[string]any{"first_name": "X", "email": "not-an-email"}}); err == nil {
		t.Fatal("invalid email accepted")
	}
	res, err := s.Run(context.Background(), org, Write, Call{Action: "create_contact", Body: map[string]any{"first_name": "Jo", "email": "JO@example.com"}})
	if err != nil || f.called(post) != 0 || !strings.Contains(res.Note, "already exists") {
		t.Fatalf("dedupe: err %v, posts %d, note %q", err, f.called(post), res.Note)
	}
	if _, err := s.Run(context.Background(), org, Write, Call{Action: "create_contact", Body: map[string]any{"first_name": "New", "email": "new@example.com"}}); err != nil || f.called(post) != 1 {
		t.Fatalf("create: err %v, posts %d", err, f.called(post))
	}
}

func TestCheckImport(t *testing.T) {
	good := "email,first_name\n\"a@example.com\",\"Smith, Ann\"\nb@example.com,Bo\n"
	if err := checkImport(good); err != nil {
		t.Fatalf("good csv: %v", err)
	}
	for name, csv := range map[string]string{
		"no email column": "name\nAnn\n",
		"invalid address": "email\nann@\n",
		"header only":     "email\n",
		"too many rows":   "email\n" + strings.Repeat("x@example.com\n", maxImportRows+1),
	} {
		if err := checkImport(csv); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRetriesOnlyWhatCannotRepeat(t *testing.T) {
	saved := retryDelays
	retryDelays = []time.Duration{0, 0}
	defer func() { retryDelays = saved }()

	f := newFake(t)
	fails := 1
	f.handlers["GET "+marketingPrefix+"/campaigns"] = func(w http.ResponseWriter, _ *http.Request) {
		if fails > 0 {
			fails--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		ok(w, []any{})
	}
	f.handlers["POST "+marketingPrefix+"/campaigns/c1/pause"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	s := f.service()
	if _, err := s.Run(context.Background(), org, Read, Call{Action: "list_campaigns"}); err != nil {
		t.Fatalf("GET not retried: %v", err)
	}
	if _, err := s.Run(context.Background(), org, Write, Call{Action: "pause_campaign", ID: "c1"}); err == nil {
		t.Fatal("want error")
	}
	if n := f.called("POST " + marketingPrefix + "/campaigns/c1/pause"); n != 1 {
		t.Fatalf("non-idempotent POST sent %d times", n)
	}
}

func TestIdempotencyKeyStable(t *testing.T) {
	f := newFake(t)
	s := f.service()
	body := map[string]any{"name": "Launch", "channel": "email"}
	for i := 0; i < 2; i++ {
		if _, err := s.Run(context.Background(), org, Write, Call{Action: "create_template", Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	k1, k2 := f.headers[0].Get("Idempotency-Key"), f.headers[1].Get("Idempotency-Key")
	if k1 == "" || k1 != k2 {
		t.Fatalf("keys %q %q", k1, k2)
	}
	if f.headers[0].Get("X-Namespace-Slug") != "workstation" {
		t.Fatal("namespace header missing")
	}
}

func TestKeyNeverInErrors(t *testing.T) {
	f := newFake(t)
	f.handlers["GET "+marketingPrefix+"/templates"] = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(w, `{"error":"bad key %s"}`, r.Header.Get("Authorization"))
	}
	_, err := f.service().Run(context.Background(), org, Read, Call{Action: "list_templates"})
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Fatalf("error leaks the key: %v", err)
	}
	if e := scrub(fmt.Errorf("dial: %s", testKey), testKey); strings.Contains(e.Error(), testKey) {
		t.Fatal("scrub left the key")
	}
}

type fakeInts []model.Integration

func (f fakeInts) ListByOrg(context.Context, uuid.UUID) ([]model.Integration, error) { return f, nil }

func opsInt(name, ns, key string, cfg map[string]any) model.Integration {
	c := map[string]any{"namespace_slug": ns}
	for k, v := range cfg {
		c[k] = v
	}
	return model.Integration{
		Name: name, Provider: model.ProviderOpsAPI, Status: "active", BaseURL: "https://ops.example.com",
		Config: c, Credentials: map[string]any{"marketing_api_key": key},
	}
}

func TestResolveKeepsTenantsApart(t *testing.T) {
	ints := fakeInts{
		opsInt("TenthMatrix", "tenthmatrix", "k-tm", map[string]any{"marketing_test_recipients": "qa@tenthmatrix.co.uk", "marketing_brand": map[string]any{"primary": "#123"}}),
		opsInt("Workstation Shop", "workstation", "k-ws", nil),
	}
	ring := RingConfig{BaseURL: "https://ring.example.com", Key: "k-ring", Namespace: "jobshout", TestRecipients: []string{"ops@jobshout.com"}, Orgs: []uuid.UUID{org}}

	c, err := resolve(context.Background(), ints, ring, org, "tenthmatrix")
	if err != nil || c.APIKey != "k-tm" || c.Namespace != "tenthmatrix" || c.Brand["primary"] != "#123" {
		t.Fatalf("by namespace: %+v %v", c, err)
	}
	if got := withRing(c, ring).TestRecipients; len(got) != 2 {
		t.Fatalf("testers %v", got)
	}
	c, err = resolve(context.Background(), ints, ring, org, "workstation shop")
	if err != nil || c.APIKey != "k-ws" {
		t.Fatalf("by name: %+v %v", c, err)
	}
	// A website the org does not have is an error, never another tenant.
	if _, err := resolve(context.Background(), ints, ring, org, "someone-else"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unknown website: %v", err)
	}
	// Two marketing workspaces and no website named: ask, don't guess.
	if _, err := resolve(context.Background(), ints, ring, org, ""); err == nil || !strings.Contains(err.Error(), "say which one") {
		t.Fatalf("ambiguous: %v", err)
	}
	// No integrations: the ring's workspace.
	c, err = resolve(context.Background(), nil, ring, org, "")
	if err != nil || c.Website != "ring" {
		t.Fatalf("ring: %+v %v", c, err)
	}
}

func TestValidEmail(t *testing.T) {
	for in, want := range map[string]bool{
		"a@example.com": true, " A@Example.COM ": true, "a@b": false, "a@.com": false,
		"Ann <a@example.com>": false, "": false, "a@@example.com": false, "a@example..com": false,
	} {
		if _, got := ValidEmail(in); got != want {
			t.Errorf("%q: got %v", in, got)
		}
	}
}

func TestOpsTableIsWellFormed(t *testing.T) {
	for _, o := range ops {
		if o.Delivers && o.Kind != Send {
			t.Errorf("%s delivers but is %s", o.Name, o.Kind)
		}
		if strings.Contains(o.Path, "approve") || strings.Contains(o.Path, "reject") || o.Method == http.MethodDelete && o.Kind != Send && !strings.Contains(o.Path, "/members/") {
			t.Errorf("%s: approvals and deletes stay with people", o.Name)
		}
	}
}

// orgInts answers ListByOrg per org, as the integrations repository does.
type orgInts map[uuid.UUID][]model.Integration

func (o orgInts) ListByOrg(_ context.Context, id uuid.UUID) ([]model.Integration, error) {
	return o[id], nil
}

func TestOneOrgNeverReachesAnothersWorkspace(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	ints := orgInts{orgA: {opsInt("TenthMatrix", "tenthmatrix", "k-tm", nil)}}
	ring := RingConfig{BaseURL: "https://ring.example.com", Key: "k-ring", Namespace: "jobshout", Orgs: []uuid.UUID{orgA}}
	ctx := context.Background()

	for _, website := range []string{"TenthMatrix", "tenthmatrix", "jobshout", ""} {
		c, err := resolve(ctx, ints, ring, orgB, website)
		if !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("org B, website %q: got %+v, %v — want no workspace", website, c, err)
		}
		if strings.Contains(err.Error(), "k-tm") || strings.Contains(err.Error(), "k-ring") {
			t.Fatalf("error leaks a key: %v", err)
		}
	}
	if c, err := resolve(ctx, ints, ring, orgA, "tenthmatrix"); err != nil || c.APIKey != "k-tm" {
		t.Fatalf("org A own website: %+v %v", c, err)
	}
	if c, err := resolve(ctx, ints, ring, orgA, "jobshout"); err != nil || c.Website != "ring" {
		t.Fatalf("org A listed for the ring: %+v %v", c, err)
	}
	// A ring with no orgs listed serves nobody.
	ring.Orgs = nil
	if _, err := resolve(ctx, nil, ring, orgA, ""); !errors.Is(err, ErrNotConfigured) || !strings.Contains(err.Error(), "OPSAPI_MARKETING_ORG_IDS") {
		t.Fatalf("unlisted ring: %v", err)
	}

	// End to end: org B's request never reaches OpsAPI.
	f := newFake(t)
	s := f.service()
	if _, err := s.Run(ctx, orgB, Read, Call{Action: "list_contacts"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("org B run: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("org B reached OpsAPI: %v", f.calls)
	}
}

func TestOrgIDsFromEnv(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got := orgIDs(" " + a.String() + ",not-a-uuid,," + b.String() + "," + uuid.Nil.String())
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("orgIDs = %v", got)
	}
}
