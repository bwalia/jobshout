package marketingemail

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/secretbox"
)

type stubRunner struct{ last StartRequest }

func (s *stubRunner) Start(_ context.Context, req StartRequest) (uuid.UUID, error) {
	s.last = req
	return uuid.New(), nil
}

func TestModule_RegistersAndIsGeneric(t *testing.T) {
	t.Cleanup(agentmodule.Reset)
	agentmodule.Register(Module(&stubRunner{}))

	m, ok := agentmodule.Lookup(model.BuiltinMarketingEmailWriter)
	if !ok {
		t.Fatal("not registered")
	}
	if m.TabSlug != "emails" || !m.StayOnTab || m.Label != model.AgentNameMarketingEmail {
		t.Errorf("module = tab %q stay %v label %q", m.TabSlug, m.StayOnTab, m.Label)
	}
	if m.ChatHint == "" || m.Seed == nil || m.Launch == nil {
		t.Error("module is missing chat hint, seed or launch")
	}

	s := agentschema.ForBuiltin(model.BuiltinMarketingEmailWriter)
	var keys []string
	for _, f := range s.Fields {
		keys = append(keys, f.Key)
		lower := strings.ToLower(f.Key)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
			t.Errorf("schema field %q looks like a credential", f.Key)
		}
	}
	want := "brief,audience,goal,primary_cta_label,primary_cta_url,products,tone,destination,publish,model"
	if got := strings.Join(keys, ","); got != want {
		t.Errorf("fields = %s\nwant     %s", got, want)
	}
	for _, f := range s.Fields {
		switch f.Key {
		case "brief":
			if !f.Required {
				t.Error("brief must be required")
			}
		case "destination":
			if f.Type != "website" || f.FormDefault != "workstation" {
				t.Errorf("destination = %+v", f)
			}
		case "publish":
			if f.Default != "live" {
				t.Errorf("publish default = %q", f.Default)
			}
		}
	}
}

func TestSeed(t *testing.T) {
	a := Seed(uuid.New())
	if a.Name != model.AgentNameMarketingEmail || a.Metadata[model.MetadataKeyBuiltin] != model.BuiltinMarketingEmailWriter {
		t.Errorf("seed = %+v", a)
	}
}

func TestLaunch_CallsStart(t *testing.T) {
	r := &stubRunner{}
	desc := "d"
	out, err := Module(r).Launch(context.Background(), agentmodule.LaunchInput{
		OrgID: uuid.New(), UserID: uuid.New(),
		Agent: &model.Agent{ID: uuid.New()},
		Task:  &model.Task{ID: uuid.New(), Description: &desc},
		Values: map[string]string{"brief": "AI shop", "goal": "announce", "primary_cta_url": "https://www.workstation.co.uk/shop",
			"publish": "draft", "destination": "workstation"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.RunID == nil || r.last.TaskID == nil || r.last.Brief.Brief != "AI shop" || r.last.Brief.Publish != "draft" || r.last.Brief.Destination != "workstation" {
		t.Errorf("out = %+v req = %+v", out, r.last)
	}
	if _, err := Module(nil).Launch(context.Background(), agentmodule.LaunchInput{}); err == nil {
		t.Error("a nil runner must be 'not configured'")
	}
}

func TestDestinations_Resolve(t *testing.T) {
	key := strings.Join([]string{"opsk", "0123456789abcdef0123456789abcdef"}, "_")
	org := uuid.New()
	list := listFake{{
		Name: "Workstation Website", Provider: model.ProviderOpsAPI, Status: "active",
		BaseURL:     "https://opsapi.workstation.co.uk/",
		Credentials: map[string]any{"api_key": key},
		Config:      map[string]any{"namespace_slug": "workstation", "site_url": "https://example.test/"},
	}}
	d := &Destinations{Integrations: list}

	for _, name := range []string{"Workstation Website", "workstation"} {
		dest, err := d.Resolve(context.Background(), org, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if dest.Namespace != "workstation" || dest.BrowserURL("a") != "https://example.test/email/a" {
			t.Errorf("%s: dest = %+v url %q", name, dest, dest.BrowserURL("a"))
		}
	}
	if _, err := d.Resolve(context.Background(), org, "nope"); err == nil {
		t.Error("an unknown website must be an error, not the ring CMS")
	}
	if _, err := d.Resolve(context.Background(), org, ""); err == nil {
		t.Error("no ring CMS and no name must be an error")
	}

	// Ring CMS: empty name, and the ring's own namespace.
	ring := opsapi.NewClient(opsapi.Config{BaseURL: "https://int-opsapi.workstation.co.uk", APIKey: key, Namespace: "ringns"})
	d = &Destinations{Ring: ring}
	for _, name := range []string{"", "ringns"} {
		dest, err := d.Resolve(context.Background(), org, name)
		if err != nil || dest.BrowserURL("a") != "https://int.workstation.co.uk/email/a" {
			t.Errorf("ring %q: %+v %v", name, dest, err)
		}
	}

	// A sealed key is refused rather than sent.
	bad := listFake{{Name: "W", Provider: model.ProviderOpsAPI, BaseURL: "https://x.example", Credentials: map[string]any{"api_key": secretbox.Prefix + "abc"}, Config: map[string]any{"namespace_slug": "w"}}}
	if _, err := (&Destinations{Integrations: bad}).Resolve(context.Background(), org, "W"); err == nil {
		t.Error("a sealed key must be refused")
	}
}

type listFake []model.Integration

func (l listFake) ListByOrg(context.Context, uuid.UUID) ([]model.Integration, error) { return l, nil }
